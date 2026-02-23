package progress

import (
	"context"
	"encoding/json"
	"fmt"

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

	beanIDs, err := proj.DB.GetTaskBeans(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans: %w", err)
	}

	// Batch fetch all bean details
	beansResult, err := proj.Beans.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get beans: %w", err)
	}

	tp := &TaskProgress{Task: task}
	for _, beanID := range beanIDs {
		status, err := proj.DB.GetTaskBeanStatus(ctx, taskID, beanID)
		if err != nil {
			return nil, fmt.Errorf("failed to get task bean status: %w", err)
		}
		if status == "" {
			status = db.StatusPending
		}
		bp := BeanProgress{ID: beanID, Status: status}
		if bean := beansResult.GetBean(beanID); bean != nil {
			bp.Title = bean.Title
			bp.Description = bean.Body
			bp.BeanStatus = bean.Status
		}
		tp.Beans = append(tp.Beans, bp)
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

	// Fetch all task beans for this work in a single query
	allTaskBeans, err := proj.DB.GetTaskBeansForWork(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans: %w", err)
	}

	// Get all work beans
	allWorkBeans, err := proj.DB.GetWorkBeans(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get work beans: %w", err)
	}

	// Get unassigned beans for this work
	unassignedWorkBeans, err := proj.DB.GetUnassignedWorkBeans(ctx, work.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get unassigned beans: %w", err)
	}

	// Collect all bean IDs for batch fetch
	beanIDSet := make(map[string]struct{})
	for _, tb := range allTaskBeans {
		beanIDSet[tb.BeanID] = struct{}{}
	}
	for _, wb := range allWorkBeans {
		beanIDSet[wb.BeanID] = struct{}{}
	}
	for _, wb := range unassignedWorkBeans {
		beanIDSet[wb.BeanID] = struct{}{}
	}
	if work.RootIssueID != "" {
		beanIDSet[work.RootIssueID] = struct{}{}
	}

	beanIDs := make([]string, 0, len(beanIDSet))
	for id := range beanIDSet {
		beanIDs = append(beanIDs, id)
	}

	// Batch fetch all bean details
	beansResult, err := proj.Beans.GetBeansWithDeps(ctx, beanIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get beans: %w", err)
	}

	// Build a map of task ID -> beans for efficient lookup
	taskBeansMap := make(map[string][]db.TaskBeanInfo)
	for _, tb := range allTaskBeans {
		taskBeansMap[tb.TaskID] = append(taskBeansMap[tb.TaskID], tb)
	}

	for _, task := range tasks {
		tp := &TaskProgress{Task: task}
		for _, tb := range taskBeansMap[task.ID] {
			status := tb.Status
			if status == "" {
				status = db.StatusPending
			}
			bp := BeanProgress{ID: tb.BeanID, Status: status}
			if bean := beansResult.GetBean(tb.BeanID); bean != nil {
				bp.Title = bean.Title
				bp.Description = bean.Body
				bp.BeanStatus = bean.Status
			}
			tp.Beans = append(tp.Beans, bp)
		}
		wp.Tasks = append(wp.Tasks, tp)
	}

	// Populate work beans
	for _, wb := range allWorkBeans {
		bp := BeanProgress{ID: wb.BeanID}
		if bean := beansResult.GetBean(wb.BeanID); bean != nil {
			bp.Title = bean.Title
			bp.Description = bean.Body
			bp.BeanStatus = bean.Status
			bp.Priority = bean.Priority
			bp.IssueType = bean.Type
		}
		wp.WorkBeans = append(wp.WorkBeans, bp)
	}

	// Ensure root issue is always available for display (it may not be in work_beans if it's an epic)
	if work.RootIssueID != "" {
		rootFound := false
		for _, wb := range wp.WorkBeans {
			if wb.ID == work.RootIssueID {
				rootFound = true
				break
			}
		}
		if !rootFound {
			if rootBean := beansResult.GetBean(work.RootIssueID); rootBean != nil {
				bp := BeanProgress{
					ID:          rootBean.ID,
					Title:       rootBean.Title,
					Description: rootBean.Body,
					BeanStatus:  rootBean.Status,
					Priority:    rootBean.Priority,
					IssueType:   rootBean.Type,
				}
				// Prepend root issue so it appears first
				wp.WorkBeans = append([]BeanProgress{bp}, wp.WorkBeans...)
			}
		}
	}

	// Populate unassigned beans (excluding root issue which is displayed separately)
	for _, wb := range unassignedWorkBeans {
		// Skip root issue - it's displayed separately in the UI
		if wb.BeanID == work.RootIssueID {
			continue
		}
		bp := BeanProgress{ID: wb.BeanID}
		if bean := beansResult.GetBean(wb.BeanID); bean != nil {
			bp.Title = bean.Title
			bp.Description = bean.Body
			bp.BeanStatus = bean.Status
			bp.Priority = bean.Priority
			bp.IssueType = bean.Type
		}
		wp.UnassignedBeans = append(wp.UnassignedBeans, bp)
	}
	wp.UnassignedBeanCount = len(wp.UnassignedBeans)

	// Get unassigned feedback bean IDs for this work
	feedbackBeanIDs, err := proj.DB.GetUnassignedFeedbackBeanIDs(ctx, work.ID)
	if err == nil {
		wp.FeedbackBeanIDs = feedbackBeanIDs
		wp.FeedbackCount = len(feedbackBeanIDs)
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
