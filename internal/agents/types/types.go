package types

// TaskType identifies the kind of task being prompted.
type TaskType string

const (
	TaskTypeImplement           TaskType = "implement"
	TaskTypeEstimate            TaskType = "estimate"
	TaskTypePR                  TaskType = "pr"
	TaskTypeReview              TaskType = "review"
	TaskTypeUpdatePRDescription TaskType = "update_pr_description"
	TaskTypePlan                TaskType = "plan"
	TaskTypeLogAnalysis         TaskType = "log_analysis"
)

// TaskParams is the single input struct for agent Run/RunInteractive.
// Not all fields are used for every TaskType.
type TaskParams struct {
	Type          TaskType
	TaskID        string
	WorkID        string
	BranchName    string
	BaseBranch    string
	BeanIDs       []string      // implement, estimate
	RootIssueID   string        // review, log_analysis
	PRURL         string        // update_pr_description
	BeanID        string        // plan
	WorkflowName  string        // log_analysis
	JobName       string        // log_analysis
	LogFilePath   string        // log_analysis
	ExistingBeads []BeadSummary // log_analysis
}

// BeadSummary provides a summary of an existing bead for matching.
type BeadSummary struct {
	ID          string
	Title       string
	Description string
}
