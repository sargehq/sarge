package feedback

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/stretchr/testify/require"
)

func TestNewFeedbackProcessor(t *testing.T) {
	client := &github.Client{}

	processor := NewFeedbackProcessor(client)
	require.NotNil(t, processor, "NewFeedbackProcessor returned nil")
	require.Equal(t, client, processor.client, "Expected client to be set")
}

func TestCategorizeCheckFailure(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name     string
		check    string
		expected github.FeedbackType
	}{
		{"Test check", "unit-tests", github.FeedbackTypeTest},
		{"Test check uppercase", "Unit-Tests", github.FeedbackTypeTest},
		{"Lint check", "eslint", github.FeedbackTypeLint},
		{"Style check", "code-style", github.FeedbackTypeLint},
		{"Build check", "build-project", github.FeedbackTypeBuild},
		{"Compile check", "compile", github.FeedbackTypeBuild},
		{"Security check", "security-scan", github.FeedbackTypeSecurity},
		{"Vulnerability check", "vulnerability-scan", github.FeedbackTypeSecurity},
		{"Generic CI", "ci-check", github.FeedbackTypeCI},
		{"Unknown check", "something-else", github.FeedbackTypeCI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.categorizeCheckFailure(tt.check)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestCategorizeWorkflowFailure(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name          string
		workflowName  string
		failureDetail string
		expected      github.FeedbackType
	}{
		{"Test workflow", "Test Suite", "unit tests failed", github.FeedbackTypeTest},
		{"Lint workflow", "Linting", "eslint errors", github.FeedbackTypeLint},
		{"Format workflow", "Code Format", "formatting issues", github.FeedbackTypeLint},
		{"Build workflow", "Build", "compilation error", github.FeedbackTypeBuild},
		{"Security workflow", "Security Scan", "vulnerabilities found", github.FeedbackTypeSecurity},
		{"Generic CI", "CI Pipeline", "step failed", github.FeedbackTypeCI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.categorizeWorkflowFailure(tt.workflowName, tt.failureDetail)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPriorityForType(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		feedbackType github.FeedbackType
		expected     int
	}{
		{github.FeedbackTypeSecurity, 0}, // Critical
		{github.FeedbackTypeBuild, 1},    // High
		{github.FeedbackTypeCI, 1},       // High
		{github.FeedbackTypeTest, 2},     // Medium
		{github.FeedbackTypeLint, 2},     // Medium
		{github.FeedbackTypeReview, 2},   // Medium
		{github.FeedbackTypeGeneral, 3},  // Low
	}

	for _, tt := range tests {
		t.Run(string(tt.feedbackType), func(t *testing.T) {
			result := processor.getPriorityForType(tt.feedbackType)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestIsActionableComment(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name       string
		body       string
		actionable bool
	}{
		{"Please request", "Please update the documentation", true},
		{"Should request", "This should be refactored", true},
		{"Must request", "You must fix this issue", true},
		{"Need to request", "We need to address this", true},
		{"Fix request", "Fix the broken test", true},
		{"TODO comment", "TODO: implement this feature", true},
		{"FIXME comment", "FIXME: memory leak here", true},
		{"Error mention", "ERROR: compilation failed", true},
		{"Failed mention", "The test failed", true},
		{"Non-actionable", "Looks good to me!", false},
		{"Simple comment", "Thanks for the PR", false},
		{"Empty comment", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.isActionableComment(tt.body)
			require.Equal(t, tt.actionable, result)
		})
	}
}

func TestTruncateText(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name     string
		text     string
		maxLen   int
		expected string
	}{
		{"Short text", "Hello", 10, "Hello"},
		{"Exact length", "Hello", 5, "Hello"},
		{"Long text", "Hello, World!", 5, "Hello..."},
		{"Empty text", "", 10, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.truncateText(tt.text, tt.maxLen)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessStatusChecks(t *testing.T) {
	processor := &FeedbackProcessor{}

	status := &github.PRStatus{
		StatusChecks: []github.StatusCheck{
			{
				Context:     "unit-tests",
				State:       "FAILURE",
				Description: "Unit tests failed",
				TargetURL:   "https://example.com/checks/1",
			},
			{
				Context:     "lint",
				State:       "ERROR",
				Description: "Linting errors found",
				TargetURL:   "https://example.com/checks/2",
			},
			{
				Context:     "build",
				State:       "SUCCESS",
				Description: "Build passed",
				TargetURL:   "https://example.com/checks/3",
			},
		},
	}

	items := processor.processStatusChecks(status)

	// Should have 2 items (the two failures)
	require.Len(t, items, 2)

	// Check first item (unit-tests failure)
	require.Equal(t, github.FeedbackTypeTest, items[0].Type)
	require.Equal(t, "Fix unit-tests failure", items[0].Title)

	// Check second item (lint error)
	require.Equal(t, github.FeedbackTypeLint, items[1].Type)
	require.Equal(t, "Fix lint failure", items[1].Title)
}

func TestProcessWorkflowRuns(t *testing.T) {
	processor := &FeedbackProcessor{
		client: &github.Client{},
	}

	status := &github.PRStatus{
		Workflows: []github.WorkflowRun{
			{
				ID:         123,
				Name:       "Test Suite",
				Status:     "completed",
				Conclusion: "failure",
				URL:        "https://example.com/runs/123",
				Jobs: []github.Job{
					{
						ID:         456,
						Name:       "Unit Tests",
						Conclusion: "failure",
						URL:        "https://example.com/jobs/456",
						Steps: []github.Step{
							{Name: "Run tests", Conclusion: "failure"},
						},
					},
				},
			},
			{
				ID:         124,
				Name:       "Build",
				Status:     "completed",
				Conclusion: "success",
				URL:        "https://example.com/runs/124",
			},
		},
	}

	// Note: This test will use generic fallback since GetJobLogs will fail
	// (we're not mocking the GitHub API). The test verifies the fallback works.
	ctx := context.Background()
	items := processor.processWorkflowRuns(ctx, "owner/repo", status)

	// Should have 1 item (the failed workflow with generic fallback)
	require.Len(t, items, 1)

	require.Equal(t, github.FeedbackTypeTest, items[0].Type)
	// Generic fallback format: "Fix {jobName}: {stepName} in {workflowName}"
	require.Equal(t, "Fix Unit Tests: Run tests in Test Suite", items[0].Title)
}

func TestProcessReviews(t *testing.T) {
	processor := &FeedbackProcessor{}

	status := &github.PRStatus{
		URL: "https://github.com/user/repo/pull/123",
		Reviews: []github.Review{
			{
				ID:     1,
				State:  "CHANGES_REQUESTED",
				Body:   "Please fix these issues",
				Author: "reviewer1",
			},
			{
				ID:     2,
				State:  "APPROVED",
				Body:   "LGTM",
				Author: "reviewer2",
			},
			{
				ID:     3,
				State:  "COMMENTED",
				Body:   "Some comments",
				Author: "reviewer3",
				Comments: []github.ReviewComment{
					{
						Path:   "file.go",
						Line:   42,
						Body:   "This needs to be fixed",
						Author: "reviewer3",
					},
				},
			},
		},
	}

	items := processor.processReviews(status)

	// Should have 2 items (CHANGES_REQUESTED and the actionable comment)
	require.Len(t, items, 2)

	// Check first item (CHANGES_REQUESTED)
	require.Equal(t, github.FeedbackTypeReview, items[0].Type)
	require.Equal(t, "Address review feedback from reviewer1", items[0].Title)
	require.Equal(t, 1, items[0].Priority)

	// Check second item (actionable comment)
	require.Equal(t, github.FeedbackTypeReview, items[1].Type)
	require.Equal(t, 2, items[1].Priority)
}

func TestCreateGenericFailureItem(t *testing.T) {
	processor := &FeedbackProcessor{
		client: &github.Client{},
	}

	workflow := github.WorkflowRun{
		ID:   123,
		Name: "CI Pipeline",
		URL:  "https://example.com/runs/123",
	}

	tests := []struct {
		name          string
		job           github.Job
		expectedTitle string
	}{
		{
			name: "Job with failed step",
			job: github.Job{
				ID:         456,
				Name:       "Test",
				Conclusion: "failure",
				URL:        "https://example.com/jobs/456",
				Steps: []github.Step{
					{Name: "Setup", Conclusion: "success"},
					{Name: "Run tests", Conclusion: "failure"},
				},
			},
			expectedTitle: "Fix Test: Run tests in CI Pipeline",
		},
		{
			name: "Job without specific failed step",
			job: github.Job{
				ID:         789,
				Name:       "Lint",
				Conclusion: "failure",
				URL:        "https://example.com/jobs/789",
				Steps:      []github.Step{},
			},
			expectedTitle: "Fix Lint in CI Pipeline",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := processor.createGenericFailureItem(workflow, tt.job)
			require.Equal(t, tt.expectedTitle, item.Title)
		})
	}
}

func TestCategorizeComment_HumanVsBot(t *testing.T) {
	client := &github.Client{}
	processor := NewFeedbackProcessor(client)

	tests := []struct {
		name             string
		author           string
		body             string
		expectedType     github.FeedbackType
		expectedPriority int
	}{
		{
			name:             "human actionable comment",
			author:           "shubsengupta",
			body:             "Please remove any markdown / implementation detail files",
			expectedType:     github.FeedbackTypeReview,
			expectedPriority: 2,
		},
		{
			name:             "bot general comment",
			author:           "github-actions[bot]",
			body:             "Some general bot message",
			expectedType:     github.FeedbackTypeGeneral,
			expectedPriority: 3,
		},
		{
			name:             "bot security comment",
			author:           "security-bot",
			body:             "Security vulnerability detected",
			expectedType:     github.FeedbackTypeSecurity,
			expectedPriority: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			comment := github.Comment{
				Author: tt.author,
				Body:   tt.body,
			}

			feedbackType := processor.categorizeComment(comment)
			require.Equal(t, tt.expectedType, feedbackType)

			priority := processor.getPriorityForType(feedbackType)
			require.Equal(t, tt.expectedPriority, priority)
		})
	}
}

func TestCategorizeComment(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name     string
		comment  github.Comment
		expected github.FeedbackType
	}{
		{
			name: "Security bot comment",
			comment: github.Comment{
				Author: "security-bot",
				Body:   "Found security vulnerability in dependencies",
			},
			expected: github.FeedbackTypeSecurity,
		},
		{
			name: "Test bot comment",
			comment: github.Comment{
				Author: "ci[bot]",
				Body:   "Test failures detected in unit tests",
			},
			expected: github.FeedbackTypeTest,
		},
		{
			name: "Lint bot comment",
			comment: github.Comment{
				Author: "linter-bot",
				Body:   "Lint errors found in files",
			},
			expected: github.FeedbackTypeLint,
		},
		{
			name: "Human comment",
			comment: github.Comment{
				Author: "user123",
				Body:   "Please fix this issue",
			},
			expected: github.FeedbackTypeReview,
		},
		{
			name: "Generic bot comment",
			comment: github.Comment{
				Author: "github-bot",
				Body:   "Automated message",
			},
			expected: github.FeedbackTypeGeneral,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.categorizeComment(tt.comment)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractTitleFromComment(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name     string
		body     string
		expected string
	}{
		{
			name:     "Short comment",
			body:     "Fix this issue",
			expected: "Fix this issue",
		},
		{
			name:     "Multi-line comment",
			body:     "Fix this issue\nHere are more details\nAnd even more",
			expected: "Fix this issue",
		},
		{
			name:     "Long first line",
			body:     "This is a very long comment that exceeds one hundred characters and should be truncated properly to fit within the limit we have set for titles",
			expected: "This is a very long comment that exceeds one hundred characters and should be truncated properly to ...",
		},
		{
			name:     "Empty comment",
			body:     "",
			expected: "Address comment feedback",
		},
		{
			name:     "Whitespace only",
			body:     "   \n  \n  ",
			expected: "Address comment feedback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.extractTitleFromComment(tt.body)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessComments(t *testing.T) {
	processor := &FeedbackProcessor{}

	status := &github.PRStatus{
		URL: "https://github.com/user/repo/pull/123",
		Comments: []github.Comment{
			{
				ID:     1,
				Author: "security-bot",
				Body:   "Security vulnerability detected: SQL injection risk",
			},
			{
				ID:     2,
				Author: "user123",
				Body:   "Thanks for the PR!",
			},
			{
				ID:     3,
				Author: "ci[bot]",
				Body:   "Test failure: 5 tests failed",
			},
		},
	}

	items := processor.processComments(status)

	// Should have 2 actionable items (security and test failure)
	require.Len(t, items, 2)

	// First should be security (higher priority)
	require.Equal(t, github.FeedbackTypeSecurity, items[0].Type)

	// Second should be test failure
	require.Equal(t, github.FeedbackTypeTest, items[1].Type)
}

func TestProcessConflicts(t *testing.T) {
	processor := &FeedbackProcessor{}

	tests := []struct {
		name           string
		mergeableState string
		expectItems    int
	}{
		{"DIRTY state returns conflict item", db.MergeableStateDirty, 1},
		{"CLEAN state returns empty", db.MergeableStateClean, 0},
		{"BLOCKED state returns empty", db.MergeableStateBlocked, 0},
		{"UNSTABLE state returns empty", db.MergeableStateUnstable, 0},
		{"Empty state returns empty", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := &github.PRStatus{
				URL:            "https://github.com/user/repo/pull/123",
				MergeableState: tt.mergeableState,
			}

			items := processor.processConflicts(status)

			require.Len(t, items, tt.expectItems)

			if tt.expectItems > 0 {
				item := items[0]
				require.Equal(t, github.FeedbackTypeConflict, item.Type)
				require.Equal(t, "Resolve merge conflicts with main", item.Title)
				require.Equal(t, 1, item.Priority)
				require.Equal(t, "merge-conflict", item.Source.ID)
			}
		})
	}
}

func TestGetPriorityForConflictType(t *testing.T) {
	processor := &FeedbackProcessor{}

	result := processor.getPriorityForType(github.FeedbackTypeConflict)
	require.Equal(t, 1, result)
}

func TestNewFeedbackProcessorWithProject(t *testing.T) {
	client := &github.Client{}

	t.Run("with nil project", func(t *testing.T) {
		processor := NewFeedbackProcessorWithProject(client, nil, "work-123")
		require.NotNil(t, processor, "NewFeedbackProcessorWithProject returned nil")
		require.Nil(t, processor.proj, "Expected proj to be nil")
		require.Equal(t, "work-123", processor.workID)
	})

	t.Run("stores all parameters", func(t *testing.T) {
		// Can't test with real project, but we can verify struct fields are set
		processor := NewFeedbackProcessorWithProject(client, nil, "w-abc")
		require.Equal(t, client, processor.client, "Expected client to be set")
		require.Equal(t, "w-abc", processor.workID)
	})
}

func TestShouldUseClaude(t *testing.T) {
	client := &github.Client{}

	t.Run("returns false when project is nil", func(t *testing.T) {
		processor := NewFeedbackProcessorWithProject(client, nil, "work-123")
		require.False(t, processor.shouldUseClaude(), "Expected shouldUseClaude() to return false when project is nil")
	})

	t.Run("returns false with basic processor", func(t *testing.T) {
		processor := NewFeedbackProcessor(client)
		require.False(t, processor.shouldUseClaude(), "Expected shouldUseClaude() to return false for basic processor")
	})
}

func TestTruncateLogContent(t *testing.T) {
	tests := []struct {
		name     string
		logs     string
		maxBytes int
		expected string
	}{
		{
			name:     "short log under limit",
			logs:     "short log",
			maxBytes: 100,
			expected: "short log",
		},
		{
			name:     "log exactly at limit",
			logs:     "exact",
			maxBytes: 5,
			expected: "exact",
		},
		{
			name:     "long log truncated to last N bytes",
			logs:     "beginning middle end",
			maxBytes: 10,
			expected: "middle end",
		},
		{
			name:     "empty log",
			logs:     "",
			maxBytes: 100,
			expected: "",
		},
		{
			name:     "truncate to single byte",
			logs:     "hello",
			maxBytes: 1,
			expected: "o",
		},
		{
			name:     "preserves error at end of log",
			logs:     "INFO: Starting build\nINFO: Compiling\nERROR: Test failed at line 42",
			maxBytes: 30,
			expected: "\nERROR: Test failed at line 42",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateLogContent(tt.logs, tt.maxBytes)
			require.Equal(t, tt.expected, result)
		})
	}
}
