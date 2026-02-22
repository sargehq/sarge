package progress

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
)

// FetchTaskPollData fetches progress data for a single task
func FetchTaskPollData(ctx context.Context, proj *project.Project, taskID string) ([]*WorkProgress, error) {
	task, err := proj.DB.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if task == nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}

	// Get the work for this task
	work, err := proj.DB.GetWork(ctx, task.WorkID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		work = &db.Work{ID: task.WorkID, Status: "unknown"}
	}

	beadIDs, err := proj.DB.GetTaskBeads(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads: %w", err)
	}

	// Batch fetch all bead details
	beadsResult, err := proj.Beads.GetBeadsWithDeps(ctx, beadIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get beads: %w", err)
	}

	tp := &TaskProgress{Task: task}
	for _, beadID := range beadIDs {
		status, err := proj.DB.GetTaskBeadStatus(ctx, taskID, beadID)
		if err != nil {
			return nil, fmt.Errorf("failed to get task bead status: %w", err)
		}
		if status == "" {
			status = db.StatusPending
		}
		bp := BeadProgress{ID: beadID, Status: status}
		if bead := beadsResult.GetBead(beadID); bead != nil {
			bp.Title = bead.Title
			bp.Description = bead.Description
			bp.BeadStatus = bead.Status
		}
		tp.Beads = append(tp.Beads, bp)
	}

	return []*WorkProgress{{
		Work:  work,
		Tasks: []*TaskProgress{tp},
	}}, nil
}

// FetchWorkPollData fetches progress data for a single work
func FetchWorkPollData(ctx context.Context, proj *project.Project, workID string) ([]*WorkProgress, error) {
	work, err := proj.DB.GetWork(ctx, workID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work: %w", err)
	}
	if work == nil {
		return nil, fmt.Errorf("work %s not found", workID)
	}

	wp, err := FetchWorkProgress(ctx, proj, work)
	if err != nil {
		return nil, err
	}
	return []*WorkProgress{wp}, nil
}

// FetchAllWorksPollData fetches progress data for all works
func FetchAllWorksPollData(ctx context.Context, proj *project.Project) ([]*WorkProgress, error) {
	allWorks, err := proj.DB.ListWorks(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to list works: %w", err)
	}

	works := make([]*WorkProgress, 0, len(allWorks))
	for _, work := range allWorks {
		wp, err := FetchWorkProgress(ctx, proj, work)
		if err != nil {
			continue // Skip works with errors
		}
		works = append(works, wp)
	}
	return works, nil
}

// FetchWorkProgress fetches progress data for a single work
func FetchWorkProgress(ctx context.Context, proj *project.Project, work *db.Work) (*WorkProgress, error) {
	wp := &WorkProgress{Work: work}

	tasks, err := proj.DB.GetWorkTasks(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks: %w", err)
	}

	// Fetch all task beads for this work in a single query
	allTaskBeads, err := proj.DB.GetTaskBeadsForWork(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads: %w", err)
	}

	// Get all work beads
	allWorkBeads, err := proj.DB.GetWorkBeads(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work beads: %w", err)
	}

	// Get unassigned beads for this work
	unassignedWorkBeads, err := proj.DB.GetUnassignedWorkBeads(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unassigned beads: %w", err)
	}

	// Collect all bead IDs for batch fetch
	beadIDSet := make(map[string]struct{})
	for _, tb := range allTaskBeads {
		beadIDSet[tb.BeadID] = struct{}{}
	}
	for _, wb := range allWorkBeads {
		beadIDSet[wb.BeadID] = struct{}{}
	}
	for _, wb := range unassignedWorkBeads {
		beadIDSet[wb.BeadID] = struct{}{}
	}
	if work.RootIssueID != "" {
		beadIDSet[work.RootIssueID] = struct{}{}
	}

	beadIDs := make([]string, 0, len(beadIDSet))
	for id := range beadIDSet {
		beadIDs = append(beadIDs, id)
	}

	// Batch fetch all bead details
	beadsResult, err := proj.Beads.GetBeadsWithDeps(ctx, beadIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get beads: %w", err)
	}

	// Build a map of task ID -> beads for efficient lookup
	taskBeadsMap := make(map[string][]db.TaskBeadInfo)
	for _, tb := range allTaskBeads {
		taskBeadsMap[tb.TaskID] = append(taskBeadsMap[tb.TaskID], tb)
	}

	for _, task := range tasks {
		tp := &TaskProgress{Task: task}
		for _, tb := range taskBeadsMap[task.ID] {
			status := tb.Status
			if status == "" {
				status = db.StatusPending
			}
			bp := BeadProgress{ID: tb.BeadID, Status: status}
			if bead := beadsResult.GetBead(tb.BeadID); bead != nil {
				bp.Title = bead.Title
				bp.Description = bead.Description
				bp.BeadStatus = bead.Status
			}
			tp.Beads = append(tp.Beads, bp)
		}
		wp.Tasks = append(wp.Tasks, tp)
	}

	// Sort tasks by numeric ID suffix descending (e.g., 14, 13, 12, ...)
	sort.Slice(wp.Tasks, func(i, j int) bool {
		return taskNumericSuffix(wp.Tasks[i].Task.ID) > taskNumericSuffix(wp.Tasks[j].Task.ID)
	})

	// Populate work beads
	for _, wb := range allWorkBeads {
		bp := BeadProgress{ID: wb.BeadID}
		if bead := beadsResult.GetBead(wb.BeadID); bead != nil {
			bp.Title = bead.Title
			bp.Description = bead.Description
			bp.BeadStatus = bead.Status
			bp.Priority = bead.Priority
			bp.IssueType = bead.Type
		}
		wp.WorkBeads = append(wp.WorkBeads, bp)
	}

	// Ensure root issue is always available for display (it may not be in work_beads if it's an epic)
	if work.RootIssueID != "" {
		rootFound := false
		for _, wb := range wp.WorkBeads {
			if wb.ID == work.RootIssueID {
				rootFound = true
				break
			}
		}
		if !rootFound {
			if rootBead := beadsResult.GetBead(work.RootIssueID); rootBead != nil {
				bp := BeadProgress{
					ID:          rootBead.ID,
					Title:       rootBead.Title,
					Description: rootBead.Description,
					BeadStatus:  rootBead.Status,
					Priority:    rootBead.Priority,
					IssueType:   rootBead.Type,
				}
				// Prepend root issue so it appears first
				wp.WorkBeads = append([]BeadProgress{bp}, wp.WorkBeads...)
			}
		}
	}

	// Populate unassigned beads (excluding root issue which is displayed separately)
	for _, wb := range unassignedWorkBeads {
		// Skip root issue - it's displayed separately in the UI
		if wb.BeadID == work.RootIssueID {
			continue
		}
		bp := BeadProgress{ID: wb.BeadID}
		if bead := beadsResult.GetBead(wb.BeadID); bead != nil {
			bp.Title = bead.Title
			bp.Description = bead.Description
			bp.BeadStatus = bead.Status
			bp.Priority = bead.Priority
			bp.IssueType = bead.Type
		}
		wp.UnassignedBeads = append(wp.UnassignedBeads, bp)
	}
	wp.UnassignedBeadCount = len(wp.UnassignedBeads)

	// Get unassigned feedback bead IDs for this work
	feedbackBeadIDs, err := proj.DB.GetUnassignedFeedbackBeadIDs(ctx, work.ID)
	if err == nil {
		wp.FeedbackBeadIDs = feedbackBeadIDs
		wp.FeedbackCount = len(feedbackBeadIDs)
	}

	// Populate PR status fields from work record
	wp.CIStatus = work.CIStatus
	wp.ApprovalStatus = work.ApprovalStatus
	wp.HasUnseenPRChanges = work.HasUnseenPRChanges
	wp.MergeableState = work.MergeableState

	// Parse approvers JSON array
	if work.Approvers != "" {
		var approvers []string
		if err := json.Unmarshal([]byte(work.Approvers), &approvers); err == nil {
			wp.Approvers = approvers
		}
	}

	return wp, nil
}

// taskNumericSuffix extracts the numeric suffix from a task ID like "work-1.14" -> 14.
// Returns 0 if the ID doesn't have a numeric suffix.
func taskNumericSuffix(id string) int {
	if idx := strings.LastIndex(id, "."); idx >= 0 {
		if n, err := strconv.Atoi(id[idx+1:]); err == nil {
			return n
		}
	}
	return 0
}
