package feedback

import (
	"context"
	"crypto/sha256"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback/logparser"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/project"
)

// maxLogContentSize is the maximum size in bytes for log content stored in task metadata.
// Logs exceeding this size are truncated, keeping the last portion (most relevant for errors).
// Since logs are written to a temp file for Claude to read (not embedded in prompt),
// this can be quite large.
const maxLogContentSize = 500 * 1024 // 500KB

// FeedbackProcessor processes PR feedback and generates actionable items.
type FeedbackProcessor struct {
	client github.ClientInterface
	// Optional fields for Claude log analysis integration
	proj   *project.Project
	workID string
}

// NewFeedbackProcessor creates a new feedback processor.
func NewFeedbackProcessor(client github.ClientInterface) *FeedbackProcessor {
	return &FeedbackProcessor{
		client: client,
	}
}

// NewFeedbackProcessorWithProject creates a feedback processor with project context.
// This enables Claude-based log analysis when configured.
func NewFeedbackProcessorWithProject(client github.ClientInterface, proj *project.Project, workID string) *FeedbackProcessor {
	return &FeedbackProcessor{
		client: client,
		proj:   proj,
		workID: workID,
	}
}

// ProcessPRFeedback fetches and processes feedback for a PR.
func (p *FeedbackProcessor) ProcessPRFeedback(ctx context.Context, prURL string) ([]github.FeedbackItem, error) {
	// Extract repo from PR URL for log fetching
	repo, err := extractRepoFromPRURL(prURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse PR URL: %w", err)
	}

	// Fetch PR status
	status, err := p.client.GetPRStatus(ctx, prURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch PR status: %w", err)
	}

	// Skip draft PRs
	if strings.EqualFold(status.State, "draft") {
		return nil, nil
	}

	var items []github.FeedbackItem

	// Process status checks
	checkItems := p.processStatusChecks(ctx, repo, status)
	items = append(items, checkItems...)

	// Process workflow runs
	workflowItems := p.processWorkflowRuns(ctx, repo, status)
	items = append(items, workflowItems...)

	// Process reviews
	reviewItems := p.processReviews(status)
	items = append(items, reviewItems...)

	// Process general comments
	commentItems := p.processComments(status)
	items = append(items, commentItems...)

	// Process merge conflicts
	conflictItems := p.processConflicts(status)
	items = append(items, conflictItems...)

	return items, nil
}

// processStatusChecks processes status check failures.
// It deduplicates against workflow runs (which provide richer feedback) and
// attempts to enrich remaining status checks with log details from their TargetURL.
func (p *FeedbackProcessor) processStatusChecks(ctx context.Context, repo string, status *github.PRStatus) []github.FeedbackItem {
	var items []github.FeedbackItem

	for _, check := range status.StatusChecks {
		if check.State == "FAILURE" || check.State == "ERROR" {
			// Deduplicate: skip status checks that have a matching workflow run,
			// since processWorkflowRuns produces richer feedback with parsed logs.
			if p.hasMatchingWorkflowRun(check.Context, status.Workflows) {
				logging.Debug("skipping status check with matching workflow run",
					"check", check.Context)
				continue
			}

			// Try to enrich with log details from TargetURL
			if enrichedItems := p.enrichStatusCheckFromTargetURL(ctx, repo, check); len(enrichedItems) > 0 {
				items = append(items, enrichedItems...)
				continue
			}

			// Fall back to generic handling
			feedbackType := p.categorizeCheckFailure(check.Context)

			// Use check description if available, otherwise provide a default
			description := check.Description
			if description == "" {
				description = fmt.Sprintf("CI check '%s' failed with state: %s", check.Context, check.State)
			}

			item := github.FeedbackItem{
				Type:        feedbackType,
				Title:       fmt.Sprintf("Fix %s failure", check.Context),
				Description: description,
				Source: github.SourceInfo{
					Type: github.SourceTypeCI,
					ID:   check.Context, // Use check name as ID for status checks
					Name: check.Context,
					URL:  check.TargetURL,
				},
				Priority: p.getPriorityForType(feedbackType),
				CICheck: &github.CICheckContext{
					CheckName: check.Context,
					State:     check.State,
				},
			}

			items = append(items, item)
		}
	}

	return items
}

// hasMatchingWorkflowRun checks if a status check has a corresponding workflow run.
// GitHub Actions report results as both status checks and workflow runs;
// we prefer the workflow run path which produces richer feedback.
//
// GitHub Actions status checks use the Context format "Workflow Name / Job Name"
// (e.g., "CI / lint"), so we match against:
// 1. Exact workflow or job name
// 2. The "workflow / job" composite format
// 3. The check name containing the job name as a component (split on " / ")
func (p *FeedbackProcessor) hasMatchingWorkflowRun(checkName string, workflows []github.WorkflowRun) bool {
	checkLower := strings.ToLower(strings.TrimSpace(checkName))

	// Split on " / " to handle GitHub Actions composite check names like "CI / lint"
	checkParts := strings.Split(checkLower, " / ")
	// Trim whitespace from each part
	for i, part := range checkParts {
		checkParts[i] = strings.TrimSpace(part)
	}

	for _, wf := range workflows {
		wfNameLower := strings.ToLower(wf.Name)

		// Exact match on workflow name
		if wfNameLower == checkLower {
			return true
		}

		for _, job := range wf.Jobs {
			jobNameLower := strings.ToLower(job.Name)

			// Exact match on job name
			if jobNameLower == checkLower {
				return true
			}

			// Match composite format: "workflow / job" in check name
			// e.g., check "CI / lint" matches workflow "CI" with job "lint"
			if len(checkParts) >= 2 {
				for _, part := range checkParts {
					if part == jobNameLower || part == wfNameLower {
						return true
					}
				}
			}

			// Match if check name contains "workflow / job" pattern
			composite := wfNameLower + " / " + jobNameLower
			if checkLower == composite {
				return true
			}
		}
	}
	return false
}

// parseGitHubActionsURL extracts repo, run ID, and job ID from a GitHub Actions URL.
// Expected format: https://github.com/owner/repo/actions/runs/RUNID/job/JOBID
// Returns empty strings if the URL doesn't match the expected format.
func parseGitHubActionsURL(targetURL string) (repo string, runID int64, jobID int64, ok bool) {
	if targetURL == "" {
		return "", 0, 0, false
	}

	// Match GitHub Actions job URLs
	re := regexp.MustCompile(`https://github\.com/([^/]+/[^/]+)/actions/runs/(\d+)/job/(\d+)`)
	matches := re.FindStringSubmatch(targetURL)
	if matches == nil {
		return "", 0, 0, false
	}

	repo = matches[1]
	runID, err1 := strconv.ParseInt(matches[2], 10, 64)
	jobID, err2 := strconv.ParseInt(matches[3], 10, 64)
	if err1 != nil || err2 != nil {
		return "", 0, 0, false
	}

	return repo, runID, jobID, true
}

// enrichStatusCheckFromTargetURL attempts to fetch logs from a GitHub Actions
// TargetURL and parse them into detailed FeedbackItems.
func (p *FeedbackProcessor) enrichStatusCheckFromTargetURL(ctx context.Context, repo string, check github.StatusCheck) []github.FeedbackItem {
	targetRepo, _, jobID, ok := parseGitHubActionsURL(check.TargetURL)
	if !ok {
		return nil
	}

	// Use the repo from the URL if available, otherwise fall back to the PR repo
	if targetRepo != "" {
		repo = targetRepo
	}

	logs, err := p.client.GetJobLogs(ctx, repo, jobID)
	if err != nil {
		logging.Debug("failed to fetch logs for status check", "check", check.Context, "error", err)
		return nil
	}

	failures, _ := logparser.ParseFailures(logs)
	if len(failures) == 0 {
		return nil
	}

	feedbackType := p.categorizeCheckFailure(check.Context)

	var items []github.FeedbackItem
	for _, f := range failures {
		shortCtx := lastPathComponent(f.Context)

		var title string
		if f.File != "" {
			if f.Column > 0 {
				title = fmt.Sprintf("Fix %s at %s:%d:%d", f.Name, f.File, f.Line, f.Column)
			} else {
				title = fmt.Sprintf("Fix %s at %s:%d", f.Name, f.File, f.Line)
			}
		} else if shortCtx != "" {
			title = fmt.Sprintf("Fix %s in %s", f.Name, shortCtx)
		} else {
			title = fmt.Sprintf("Fix %s", f.Name)
		}

		sourceID := generateFailureSourceID(f.Name, f.File, f.Line, f.Message)

		item := github.FeedbackItem{
			Type:        feedbackType,
			Title:       title,
			Description: formatFailure(f),
			Source: github.SourceInfo{
				Type: github.SourceTypeCI,
				ID:   sourceID,
				Name: check.Context,
				URL:  check.TargetURL,
			},
			Priority: p.getPriorityForType(feedbackType),
			CICheck: &github.CICheckContext{
				CheckName: check.Context,
				State:     check.State,
			},
		}

		items = append(items, item)
	}

	return items
}

// processWorkflowRuns processes workflow run failures.
func (p *FeedbackProcessor) processWorkflowRuns(ctx context.Context, repo string, status *github.PRStatus) []github.FeedbackItem {
	var items []github.FeedbackItem

	for _, workflow := range status.Workflows {
		if workflow.Conclusion == "failure" {
			for _, job := range workflow.Jobs {
				if job.Conclusion == "failure" {
					// Try to get detailed failures for test or lint jobs
					if isTestJob(job.Name) || isLintJob(job.Name) {
						// Check if Claude-based log analysis is enabled
						if p.shouldUseClaude() {
							// Check if we've already created a task for this specific job
							// Job IDs are unique per CI run, so this prevents duplicate analysis
							existingTaskID, err := p.findExistingLogAnalysisTaskByJobID(ctx, job.ID)
							if err != nil {
								logging.Warn("failed to check for existing log_analysis task", "job_id", job.ID, "error", err)
							}
							if existingTaskID != "" {
								logging.Debug("skipping log fetch - log_analysis task already exists",
									"existing_task_id", existingTaskID,
									"job_id", job.ID,
									"workflow", workflow.Name,
									"job", job.Name)
								continue // Skip this job entirely - already being analyzed
							}
						}

						logs, err := p.client.GetJobLogs(ctx, repo, job.ID)
						if err == nil {
							// Check if Claude-based log analysis is enabled
							if p.shouldUseClaude() {
								// Create a log_analysis task instead of parsing inline
								if taskID, err := p.createLogAnalysisTask(ctx, workflow, job, logs); err == nil {
									logging.Debug("scheduled log_analysis task for Claude",
										"task_id", taskID,
										"workflow", workflow.Name,
										"job", job.Name,
										"job_id", job.ID)
									continue // Skip further processing for this job
								}
								// If task creation fails, fall through to Go-based parsing
							}

							failures, _ := logparser.ParseFailures(logs)
							if len(failures) > 0 {
								for _, f := range failures {
									items = append(items, p.createFailureItem(workflow, job, f))
								}
								continue // Skip generic handling
							}
						}
					}
					// Fall back to generic handling
					items = append(items, p.createGenericFailureItem(workflow, job))
				}
			}
		}
	}

	return items
}

// shouldUseClaude returns true if Claude-based log analysis is enabled and configured.
func (p *FeedbackProcessor) shouldUseClaude() bool {
	if p.proj == nil {
		return false
	}
	return p.proj.Config.LogParser.ShouldUseClaude()
}

// createLogAnalysisTask creates a log_analysis task for Claude to process.
// Returns the task ID on success.
// Note: Caller should check for existing tasks via findExistingLogAnalysisTaskByJobID before calling.
func (p *FeedbackProcessor) createLogAnalysisTask(ctx context.Context, workflow github.WorkflowRun, job github.Job, logs string) (string, error) {
	if p.proj == nil || p.workID == "" {
		return "", fmt.Errorf("project context not available for log analysis task creation")
	}

	// Get work details for root issue ID
	work, err := p.proj.DB.GetWork(ctx, p.workID)
	if err != nil {
		return "", fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return "", fmt.Errorf("work not found for ID: %s", p.workID)
	}

	// Generate task ID
	taskNum, err := p.proj.DB.GetNextTaskNumber(ctx, p.workID)
	if err != nil {
		return "", fmt.Errorf("failed to get next task number: %w", err)
	}
	taskID := fmt.Sprintf("%s.%d", p.workID, taskNum)

	// Create the task (no beads - Claude will create them)
	if err := p.proj.DB.CreateTask(ctx, taskID, "log_analysis", nil, 0, p.workID); err != nil {
		return "", fmt.Errorf("failed to create log_analysis task: %w", err)
	}

	// Store log analysis parameters as task metadata
	// Truncate log content to prevent database issues with very large CI logs
	truncatedLogs := truncateLogContent(logs, maxLogContentSize)

	metadata := map[string]string{
		"workflow_name": workflow.Name,
		"job_name":      job.Name,
		"job_id":        fmt.Sprintf("%d", job.ID), // Used for deduplication
		"branch_name":   work.BranchName,
		"root_issue_id": work.RootIssueID,
		"log_content":   truncatedLogs,
	}

	for key, value := range metadata {
		if err := p.proj.DB.SetTaskMetadata(ctx, taskID, key, value); err != nil {
			// Log warning but don't fail the task creation
			logging.Warn("failed to set metadata for task", "key", key, "task_id", taskID, "error", err)
		}
	}

	return taskID, nil
}

// findExistingLogAnalysisTaskByJobID checks if a log_analysis task already exists for this CI job.
// Job IDs are unique per CI run, so this prevents duplicate analysis of the same failing job.
// Returns the task ID if found, empty string otherwise.
func (p *FeedbackProcessor) findExistingLogAnalysisTaskByJobID(ctx context.Context, jobID int64) (string, error) {
	if p.proj == nil || p.workID == "" {
		return "", nil
	}

	// Get all tasks for this work
	tasks, err := p.proj.DB.GetWorkTasks(ctx, p.workID)
	if err != nil {
		return "", fmt.Errorf("failed to get work tasks: %w", err)
	}

	jobIDStr := fmt.Sprintf("%d", jobID)

	// Check each log_analysis task for matching job_id metadata
	// Any status counts - same job_id means same CI run, same logs
	for _, task := range tasks {
		if task.TaskType != "log_analysis" {
			continue
		}

		// Check metadata for matching job_id
		taskJobID, err := p.proj.DB.GetTaskMetadata(ctx, task.ID, "job_id")
		if err != nil {
			continue // Skip if we can't read metadata
		}

		if taskJobID == jobIDStr {
			return task.ID, nil
		}
	}

	return "", nil
}

// createFailureItem creates a FeedbackItem for a specific failure.
func (p *FeedbackProcessor) createFailureItem(workflow github.WorkflowRun, job github.Job, f logparser.Failure) github.FeedbackItem {
	shortCtx := lastPathComponent(f.Context)

	// Determine title based on whether we have file/line info
	var title string
	if f.File != "" {
		if f.Column > 0 {
			title = fmt.Sprintf("Fix %s at %s:%d:%d", f.Name, f.File, f.Line, f.Column)
		} else {
			title = fmt.Sprintf("Fix %s at %s:%d", f.Name, f.File, f.Line)
		}
	} else if shortCtx != "" {
		title = fmt.Sprintf("Fix %s in %s", f.Name, shortCtx)
	} else {
		title = fmt.Sprintf("Fix %s", f.Name)
	}

	// Determine feedback type based on job name
	feedbackType := github.FeedbackTypeTest
	if isLintJob(job.Name) {
		feedbackType = github.FeedbackTypeLint
	}

	// Generate content-based source ID for deduplication across CI runs
	sourceID := generateFailureSourceID(f.Name, f.File, f.Line, f.Message)

	return github.FeedbackItem{
		Type:        feedbackType,
		Title:       title,
		Description: formatFailure(f),
		Source: github.SourceInfo{
			Type: github.SourceTypeWorkflow,
			ID:   sourceID,
			Name: workflow.Name,
			URL:  job.URL,
		},
		Priority: p.getPriorityForType(feedbackType),
		Workflow: &github.WorkflowContext{
			WorkflowName:  workflow.Name,
			FailureDetail: f.Name,
			RunID:         workflow.ID,
			JobName:       job.Name,
		},
	}
}

// createGenericFailureItem creates a FeedbackItem for a generic job failure.
func (p *FeedbackProcessor) createGenericFailureItem(workflow github.WorkflowRun, job github.Job) github.FeedbackItem {
	// Try to find the specific failed step
	failedStep := ""
	for _, step := range job.Steps {
		if step.Conclusion == "failure" {
			failedStep = step.Name
			break
		}
	}

	detail := job.Name
	if failedStep != "" {
		detail = fmt.Sprintf("%s: %s", job.Name, failedStep)
	}

	feedbackType := p.categorizeWorkflowFailure(workflow.Name, detail)
	jobName, stepName := parseWorkflowDetail(detail)

	// Generate content-based source ID for deduplication across CI runs
	sourceID := generateGenericFailureSourceID(workflow.Name, jobName, stepName)

	return github.FeedbackItem{
		Type:        feedbackType,
		Title:       fmt.Sprintf("Fix %s in %s", detail, workflow.Name),
		Description: fmt.Sprintf("Workflow '%s' failed at: %s", workflow.Name, detail),
		Source: github.SourceInfo{
			Type: github.SourceTypeWorkflow,
			ID:   sourceID,
			Name: workflow.Name,
			URL:  job.URL,
		},
		Priority: p.getPriorityForType(feedbackType),
		Workflow: &github.WorkflowContext{
			WorkflowName:  workflow.Name,
			FailureDetail: detail,
			RunID:         workflow.ID,
			JobName:       jobName,
			StepName:      stepName,
		},
	}
}

// formatFailure formats a failure for display.
func formatFailure(f logparser.Failure) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("`%s` failed", f.Name))
	if f.File != "" {
		if f.Column > 0 {
			sb.WriteString(fmt.Sprintf(" at %s:%d:%d", f.File, f.Line, f.Column))
		} else {
			sb.WriteString(fmt.Sprintf(" at %s:%d", f.File, f.Line))
		}
	}
	sb.WriteString("\n\n")
	if f.Message != "" {
		sb.WriteString(fmt.Sprintf("**Error:** %s\n\n", f.Message))
	}
	if f.RawOutput != "" {
		sb.WriteString("```\n")
		sb.WriteString(f.RawOutput)
		sb.WriteString("\n```")
	}
	return sb.String()
}

// isTestJob returns true if the job name suggests it runs tests.
func isTestJob(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "test")
}

// isLintJob returns true if the job name suggests it runs linting.
func isLintJob(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "lint") || strings.Contains(lower, "format") || strings.Contains(lower, "style")
}

// lastPathComponent returns the last component of a path.
func lastPathComponent(path string) string {
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}

// extractRepoFromPRURL extracts the owner/repo from a PR URL.
func extractRepoFromPRURL(prURL string) (string, error) {
	// Expected format: https://github.com/owner/repo/pull/123
	parts := strings.Split(prURL, "/")
	if len(parts) < 5 {
		return "", fmt.Errorf("invalid PR URL format: %s", prURL)
	}
	// Find github.com in the URL and extract owner/repo
	for i, part := range parts {
		if part == "github.com" && i+2 < len(parts) {
			return fmt.Sprintf("%s/%s", parts[i+1], parts[i+2]), nil
		}
	}
	return "", fmt.Errorf("could not extract repo from PR URL: %s", prURL)
}

// parseWorkflowDetail extracts job and step names from failure detail.
// Format is either "jobName: stepName" or just "jobName".
func parseWorkflowDetail(detail string) (jobName, stepName string) {
	if idx := strings.Index(detail, ": "); idx != -1 {
		return detail[:idx], detail[idx+2:]
	}
	return detail, ""
}

// processReviews processes review comments.
func (p *FeedbackProcessor) processReviews(status *github.PRStatus) []github.FeedbackItem {
	var items []github.FeedbackItem

	for _, review := range status.Reviews {
		// Process reviews requesting changes
		if review.State == "CHANGES_REQUESTED" {
			item := github.FeedbackItem{
				Type:        github.FeedbackTypeReview,
				Title:       fmt.Sprintf("Address review feedback from %s", review.Author),
				Description: review.Body,
				Source: github.SourceInfo{
					Type: github.SourceTypeReviewComment,
					ID:   fmt.Sprintf("%d", review.ID),
					Name: review.Author,
					URL:  status.URL, // Link to PR
				},
				Priority: 1, // High priority for requested changes
				Review: &github.ReviewContext{
					Reviewer:  review.Author,
					CommentID: int64(review.ID),
				},
			}

			items = append(items, item)
		}

		// Process specific review comments - ALL review comments are considered actionable
		for _, comment := range review.Comments {
			// Skip only trivial comments like "LGTM", "looks good", etc.
			if p.isTrivialComment(comment.Body) {
				continue
			}

			// Create a unique URL for this review comment
			// GitHub review comments have a different URL structure than issue comments
			commentURL := fmt.Sprintf("%s#discussion_r%d", status.URL, comment.ID)

			// Use Line if available, otherwise fall back to OriginalLine
			lineNum := comment.Line
			if lineNum == 0 {
				lineNum = comment.OriginalLine
			}

			item := github.FeedbackItem{
				Type:        github.FeedbackTypeReview,
				Title:       fmt.Sprintf("Fix issue in %s (line %d)", comment.Path, lineNum),
				Description: comment.Body,
				Source: github.SourceInfo{
					Type: github.SourceTypeReviewComment,
					ID:   fmt.Sprintf("%d", comment.ID),
					Name: comment.Author,
					URL:  commentURL,
				},
				Priority: 2, // Medium priority for line comments
				Review: &github.ReviewContext{
					File:        comment.Path,
					Line:        lineNum,
					Reviewer:    comment.Author,
					CommentID:   int64(comment.ID),
					InReplyToID: int64(comment.InReplyToID),
				},
			}

			items = append(items, item)
		}
	}

	return items
}

// processComments processes general PR comments.
func (p *FeedbackProcessor) processComments(status *github.PRStatus) []github.FeedbackItem {
	var items []github.FeedbackItem

	for _, comment := range status.Comments {
		if p.isActionableComment(comment.Body) {
			feedbackType := p.categorizeComment(comment)

			// Create a unique URL for this issue comment
			commentURL := fmt.Sprintf("%s#issuecomment-%d", status.URL, comment.ID)

			item := github.FeedbackItem{
				Type:        feedbackType,
				Title:       p.extractTitleFromComment(comment.Body),
				Description: comment.Body,
				Source: github.SourceInfo{
					Type: github.SourceTypeIssueComment,
					ID:   fmt.Sprintf("%d", comment.ID),
					Name: comment.Author,
					URL:  commentURL,
				},
				Priority: p.getPriorityForType(feedbackType),
				IssueComment: &github.IssueCommentContext{
					Author:    comment.Author,
					CommentID: int64(comment.ID),
				},
			}

			items = append(items, item)
		}
	}

	return items
}

// processConflicts checks for merge conflicts in the PR.
func (p *FeedbackProcessor) processConflicts(status *github.PRStatus) []github.FeedbackItem {
	var items []github.FeedbackItem

	// GitHub returns mergeStateStatus="DIRTY" for PRs with conflicts
	if status.MergeableState == db.MergeableStateDirty {
		item := github.FeedbackItem{
			Type:        github.FeedbackTypeConflict,
			Title:       "Resolve merge conflicts with main",
			Description: "This branch has merge conflicts that must be resolved. Merge main into this branch and resolve any conflicts.",
			Source: github.SourceInfo{
				Type: github.SourceTypeCI,
				ID:   "merge-conflict",
				Name: "Merge Conflict",
				URL:  status.URL,
			},
			Priority: 1,
		}
		items = append(items, item)
	}

	return items
}

// Helper functions

func (p *FeedbackProcessor) categorizeCheckFailure(checkName string) github.FeedbackType {
	lower := strings.ToLower(checkName)

	if strings.Contains(lower, "test") {
		return github.FeedbackTypeTest
	} else if strings.Contains(lower, "lint") || strings.Contains(lower, "style") {
		return github.FeedbackTypeLint
	} else if strings.Contains(lower, "build") || strings.Contains(lower, "compile") {
		return github.FeedbackTypeBuild
	} else if strings.Contains(lower, "security") || strings.Contains(lower, "vulnerability") {
		return github.FeedbackTypeSecurity
	}

	return github.FeedbackTypeCI
}

func (p *FeedbackProcessor) categorizeWorkflowFailure(workflowName, failureDetail string) github.FeedbackType {
	lower := strings.ToLower(workflowName + " " + failureDetail)

	if strings.Contains(lower, "test") {
		return github.FeedbackTypeTest
	} else if strings.Contains(lower, "lint") || strings.Contains(lower, "format") {
		return github.FeedbackTypeLint
	} else if strings.Contains(lower, "build") || strings.Contains(lower, "compile") {
		return github.FeedbackTypeBuild
	} else if strings.Contains(lower, "security") || strings.Contains(lower, "scan") {
		return github.FeedbackTypeSecurity
	}

	return github.FeedbackTypeCI
}

func (p *FeedbackProcessor) isActionableComment(body string) bool {
	// Filter out sarge's own comments to prevent feedback loops
	// Sarge posts comments with a hidden marker that we can detect
	if strings.Contains(body, "<!-- sarge-bot -->") {
		return false
	}

	// Check for patterns that indicate actionable feedback
	actionablePatterns := []string{
		"please",
		"should",
		"must",
		"need to",
		"needs to",
		"fix",
		"change",
		"update",
		"add",
		"remove",
		"todo",
		"fixme",
		"bug",
		"error",
		"warning",
		"failed",
		"failure",
		"detected",
		"vulnerability",
		"risk",
	}

	lower := strings.ToLower(body)
	for _, pattern := range actionablePatterns {
		if strings.Contains(lower, pattern) {
			return true
		}
	}

	return false
}

func (p *FeedbackProcessor) isTrivialComment(body string) bool {
	// Filter out only truly trivial comments
	trivialPatterns := []string{
		"lgtm",
		"looks good to me",
		"looks good",
		"nice",
		"great",
		"thanks",
		"thank you",
		"+1",
		"👍",
		"approved",
		"ship it",
	}

	trimmed := strings.TrimSpace(strings.ToLower(body))

	// Check if the entire comment is just a trivial phrase
	for _, pattern := range trivialPatterns {
		if trimmed == pattern || trimmed == pattern+"!" || trimmed == pattern+"." {
			return true
		}
	}

	// Very short comments (less than 10 chars) that don't contain actionable content
	if len(trimmed) < 10 && !strings.Contains(trimmed, "fix") && !strings.Contains(trimmed, "bug") {
		return true
	}

	return false
}

func (p *FeedbackProcessor) categorizeComment(comment github.Comment) github.FeedbackType {
	lower := strings.ToLower(comment.Body)

	// Check for bot comments with specific patterns
	if strings.Contains(comment.Author, "bot") || strings.Contains(comment.Author, "[bot]") {
		if strings.Contains(lower, "security") || strings.Contains(lower, "vulnerability") {
			return github.FeedbackTypeSecurity
		} else if strings.Contains(lower, "test") && strings.Contains(lower, "fail") {
			return github.FeedbackTypeTest
		} else if strings.Contains(lower, "lint") || strings.Contains(lower, "style") {
			return github.FeedbackTypeLint
		}
		// Bot comments that don't match specific patterns are still general (priority 3)
		return github.FeedbackTypeGeneral
	}

	// Human-written comments should be treated as review-level feedback (priority 2)
	// since they represent actionable feedback from team members
	return github.FeedbackTypeReview
}

func (p *FeedbackProcessor) extractTitleFromComment(body string) string {
	// Try to extract a meaningful title from the comment
	lines := strings.Split(body, "\n")
	if len(lines) > 0 {
		firstLine := strings.TrimSpace(lines[0])
		if firstLine != "" {
			if len(firstLine) > 100 {
				return firstLine[:100] + "..."
			}
			return firstLine
		}
	}
	return "Address comment feedback"
}

func (p *FeedbackProcessor) getPriorityForType(feedbackType github.FeedbackType) int {
	switch feedbackType {
	case github.FeedbackTypeSecurity:
		return 0 // Critical
	case github.FeedbackTypeBuild, github.FeedbackTypeCI, github.FeedbackTypeConflict:
		return 1 // High - conflicts block merging
	case github.FeedbackTypeTest:
		return 2 // Medium
	case github.FeedbackTypeLint, github.FeedbackTypeReview:
		return 2 // Medium
	default:
		return 3 // Low
	}
}

// truncateLogContent truncates log content to the specified maximum size.
// It keeps the last maxBytes of the log, as the end typically contains the most
// relevant error information.
func truncateLogContent(logs string, maxBytes int) string {
	if len(logs) <= maxBytes {
		return logs
	}
	// Keep the last maxBytes - error details are usually at the end
	return logs[len(logs)-maxBytes:]
}

// Patterns for normalizing content to enable stable hashing
var (
	// Timestamps: 2024-01-15 10:30:45, 2024/01/15 10:30:45, Jan 15 10:30:45
	timestampPattern = regexp.MustCompile(`\d{4}[-/]\d{2}[-/]\d{2}[T ]?\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?`)
	// Short timestamps: 10:30:45
	shortTimePattern = regexp.MustCompile(`\d{2}:\d{2}:\d{2}(?:\.\d+)?`)
	// Memory addresses: 0x7fff5fbff8c0
	memoryAddrPattern = regexp.MustCompile(`0x[0-9a-fA-F]+`)
	// Temp paths: /tmp/..., /var/folders/..., C:\Users\...\AppData\Local\Temp\...
	tempPathPattern = regexp.MustCompile(`(?:/tmp/[^\s:]+|/var/folders/[^\s:]+|C:\\[^:]*\\Temp\\[^\s:]+)`)
	// Process IDs: pid=12345, pid: 12345, PID 12345
	pidPattern = regexp.MustCompile(`(?i)pid[=:\s]+\d+`)
	// Goroutine IDs: goroutine 123
	goroutinePattern = regexp.MustCompile(`goroutine \d+`)
	// Duration values: (1.234s), 1.234ms, 1234µs
	durationPattern = regexp.MustCompile(`\(\d+\.?\d*[µmn]?s\)`)
	// Stack trace line numbers from other packages (not the test file itself)
	// e.g., /usr/local/go/src/runtime/panic.go:123
	externalLinePattern = regexp.MustCompile(`(?:/usr/local/go/|/go/pkg/)[^\s:]+:\d+`)
)

// normalizeContent strips variable content from error messages to enable stable hashing.
// This allows the same logical failure to produce the same hash across different CI runs,
// even if timestamps, memory addresses, or other transient values change.
func normalizeContent(content string) string {
	normalized := content

	// Strip timestamps
	normalized = timestampPattern.ReplaceAllString(normalized, "")
	normalized = shortTimePattern.ReplaceAllString(normalized, "")

	// Strip memory addresses
	normalized = memoryAddrPattern.ReplaceAllString(normalized, "")

	// Strip temp paths
	normalized = tempPathPattern.ReplaceAllString(normalized, "")

	// Strip process IDs
	normalized = pidPattern.ReplaceAllString(normalized, "")

	// Strip goroutine IDs
	normalized = goroutinePattern.ReplaceAllString(normalized, "")

	// Strip duration values (which vary between runs)
	normalized = durationPattern.ReplaceAllString(normalized, "")

	// Strip external stack trace line numbers
	normalized = externalLinePattern.ReplaceAllString(normalized, "")

	// Collapse whitespace
	normalized = strings.Join(strings.Fields(normalized), " ")

	return strings.TrimSpace(normalized)
}

// generateFailureSourceID creates a content-based source ID for a parsed failure.
// This produces stable IDs across CI runs for the same logical failure.
func generateFailureSourceID(failureName, file string, line int, message string) string {
	// Build content string from stable failure attributes
	// Include file and line from the test itself (not stack traces)
	content := fmt.Sprintf("%s|%s:%d|%s", failureName, file, line, normalizeContent(message))
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("fail-%x", hash[:8])
}

// generateGenericFailureSourceID creates a content-based source ID for a generic workflow failure.
// This produces stable IDs across CI runs for the same logical failure.
func generateGenericFailureSourceID(workflowName, jobName, stepName string) string {
	content := fmt.Sprintf("%s|%s|%s", workflowName, jobName, normalizeContent(stepName))
	hash := sha256.Sum256([]byte(content))
	return fmt.Sprintf("fail-%x", hash[:8])
}
