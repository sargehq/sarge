package types

// BeadSummary provides a summary of an existing bead for matching.
type BeadSummary struct {
	ID          string
	Title       string
	Description string
}

// LogAnalysisParams contains parameters for building a log analysis prompt.
type LogAnalysisParams struct {
	TaskID        string
	WorkID        string
	BranchName    string
	RootIssueID   string
	WorkflowName  string
	JobName       string
	LogFilePath   string        // Path to temp file containing CI logs
	ExistingBeads []BeadSummary // Existing open beads to match against
}
