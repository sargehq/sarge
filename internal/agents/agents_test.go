package agents

import (
	"strings"
	"testing"

	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/project"
	"github.com/stretchr/testify/require"
)

func TestBuildLogAnalysisPrompt(t *testing.T) {
	agent := NewAgent(nil)

	tests := []struct {
		name           string
		params         types.LogAnalysisParams
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "Full parameters",
			params: types.LogAnalysisParams{
				TaskID:       "w-abc.5",
				WorkID:       "w-abc",
				BranchName:   "feature/test-branch",
				RootIssueID:  "beads-123",
				WorkflowName: "CI Pipeline",
				JobName:      "Unit Tests",
				LogFilePath:  "--- FAIL: TestSomething (0.02s)",
			},
			wantContains: []string{
				"Work w-abc",
				"Branch: feature/test-branch",
				"Job: Unit Tests",
				"Workflow: CI Pipeline",
				"--- FAIL: TestSomething (0.02s)",
				"sarge complete w-abc.5",
				"--parent beads-123",
			},
		},
		{
			name: "Without root issue ID",
			params: types.LogAnalysisParams{
				TaskID:       "w-xyz.1",
				WorkID:       "w-xyz",
				BranchName:   "main",
				RootIssueID:  "",
				WorkflowName: "Build",
				JobName:      "Compile",
				LogFilePath:  "compilation error: undefined reference",
			},
			wantContains: []string{
				"Work w-xyz",
				"Branch: main",
				"Job: Compile",
				"Workflow: Build",
				"compilation error: undefined reference",
				"sarge complete w-xyz.1",
			},
			wantNotContain: []string{
				"--parent",
			},
		},
		{
			name: "Empty log file path",
			params: types.LogAnalysisParams{
				TaskID:       "w-test.2",
				WorkID:       "w-test",
				BranchName:   "dev",
				RootIssueID:  "beads-456",
				WorkflowName: "Tests",
				JobName:      "Integration",
				LogFilePath:  "",
			},
			wantContains: []string{
				"Work w-test",
				"Job: Integration",
				"REQUIRED: Mark Task Complete",
			},
		},
		{
			name: "Log file path included",
			params: types.LogAnalysisParams{
				TaskID:       "w-multi.3",
				WorkID:       "w-multi",
				BranchName:   "feature/multiline",
				RootIssueID:  "",
				WorkflowName: "CI",
				JobName:      "Test",
				LogFilePath:  "/tmp/ci-log-12345.txt",
			},
			wantContains: []string{
				"/tmp/ci-log-12345.txt",
				"sarge complete w-multi.3",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := agent.BuildLogAnalysisPrompt(tt.params)

			for _, want := range tt.wantContains {
				require.Contains(t, result, want, "BuildLogAnalysisPrompt() missing expected content")
			}

			for _, notWant := range tt.wantNotContain {
				require.NotContains(t, result, notWant, "BuildLogAnalysisPrompt() contains unexpected content")
			}
		})
	}
}

func TestLogAnalysisParams(t *testing.T) {
	// Test that the struct can be properly initialized
	params := types.LogAnalysisParams{
		TaskID:       "task-1",
		WorkID:       "work-1",
		BranchName:   "main",
		RootIssueID:  "issue-1",
		WorkflowName: "workflow-1",
		JobName:      "job-1",
		LogFilePath:  "content",
	}

	require.Equal(t, "task-1", params.TaskID)
	require.Equal(t, "work-1", params.WorkID)
	require.Equal(t, "main", params.BranchName)
	require.Equal(t, "issue-1", params.RootIssueID)
	require.Equal(t, "workflow-1", params.WorkflowName)
	require.Equal(t, "job-1", params.JobName)
	require.Equal(t, "content", params.LogFilePath)
}

func TestBuildLogAnalysisPromptPriorityGuidelines(t *testing.T) {
	agent := NewAgent(nil)

	// Verify the prompt includes priority guidelines
	params := types.LogAnalysisParams{
		TaskID:       "w-test.1",
		WorkID:       "w-test",
		BranchName:   "main",
		WorkflowName: "CI",
		JobName:      "Test",
		LogFilePath:  "test failure",
	}

	result := agent.BuildLogAnalysisPrompt(params)

	// Check that priority guidelines are included
	priorities := []string{
		"P0",
		"P1",
		"P2",
		"P3",
	}

	for _, p := range priorities {
		require.True(t, strings.Contains(result, p), "BuildLogAnalysisPrompt() missing priority guideline: %s", p)
	}
}

func TestBuildLogAnalysisPromptBdCreateCommand(t *testing.T) {
	agent := NewAgent(nil)

	// Verify the prompt includes bd create command examples
	params := types.LogAnalysisParams{
		TaskID:       "w-test.1",
		WorkID:       "w-test",
		BranchName:   "main",
		WorkflowName: "CI",
		JobName:      "Test",
		LogFilePath:  "test failure",
	}

	result := agent.BuildLogAnalysisPrompt(params)

	// Check that bd create command format is included
	require.Contains(t, result, "bd create", "BuildLogAnalysisPrompt() missing bd create command")

	// Check that it includes type options
	require.Contains(t, result, "--type", "BuildLogAnalysisPrompt() missing --type flag")

	// Check that it includes priority option
	require.Contains(t, result, "--priority", "BuildLogAnalysisPrompt() missing --priority flag")
}

func TestLogAnalysisParamsExistingBeadsField(t *testing.T) {
	// Test that ExistingBeads field is properly included in the struct
	params := types.LogAnalysisParams{
		TaskID:       "task-1",
		WorkID:       "work-1",
		BranchName:   "main",
		RootIssueID:  "issue-1",
		WorkflowName: "workflow-1",
		JobName:      "job-1",
		LogFilePath:  "content",
		ExistingBeads: []types.BeadSummary{
			{ID: "bead-1", Title: "Fix test failure", Description: "Test failed at file.go:42"},
			{ID: "bead-2", Title: "Lint error", Description: "Missing comment on exported function"},
		},
	}

	require.Len(t, params.ExistingBeads, 2)
	require.Equal(t, "bead-1", params.ExistingBeads[0].ID)
	require.Equal(t, "Fix test failure", params.ExistingBeads[0].Title)
	require.Equal(t, "Test failed at file.go:42", params.ExistingBeads[0].Description)
}

func TestBuildLogAnalysisPromptWithExistingBeads(t *testing.T) {
	agent := NewAgent(nil)

	t.Run("renders existing beads section", func(t *testing.T) {
		params := types.LogAnalysisParams{
			TaskID:       "w-test.1",
			WorkID:       "w-test",
			BranchName:   "main",
			WorkflowName: "CI",
			JobName:      "Test",
			LogFilePath:  "--- FAIL: TestSomething (0.02s)",
			ExistingBeads: []types.BeadSummary{
				{ID: "beads-123", Title: "Fix TestUserAuth failure", Description: "Test failed at auth_test.go:42"},
				{ID: "beads-456", Title: "Fix lint error in utils", Description: "Missing comment"},
			},
		}

		result := agent.BuildLogAnalysisPrompt(params)

		// Should contain the existing beads section header
		require.Contains(t, result, "Existing Open Issues")

		// Should contain bead IDs and titles
		require.Contains(t, result, "beads-123")
		require.Contains(t, result, "Fix TestUserAuth failure")
		require.Contains(t, result, "beads-456")
		require.Contains(t, result, "Fix lint error in utils")

		// Should contain descriptions
		require.Contains(t, result, "Test failed at auth_test.go:42")
		require.Contains(t, result, "Missing comment")

		// Should contain instructions to check existing beads
		require.Contains(t, result, "matches an existing issue")
		require.Contains(t, result, "skip it")
	})

	t.Run("no existing beads section when empty", func(t *testing.T) {
		params := types.LogAnalysisParams{
			TaskID:        "w-test.1",
			WorkID:        "w-test",
			BranchName:    "main",
			WorkflowName:  "CI",
			JobName:       "Test",
			LogFilePath:   "--- FAIL: TestSomething (0.02s)",
			ExistingBeads: nil,
		}

		result := agent.BuildLogAnalysisPrompt(params)

		// Should not contain the existing beads section header when no beads
		require.NotContains(t, result, "Existing Open Issues")
	})

	t.Run("handles beads without description", func(t *testing.T) {
		params := types.LogAnalysisParams{
			TaskID:       "w-test.1",
			WorkID:       "w-test",
			BranchName:   "main",
			WorkflowName: "CI",
			JobName:      "Test",
			LogFilePath:  "--- FAIL: TestSomething (0.02s)",
			ExistingBeads: []types.BeadSummary{
				{ID: "beads-789", Title: "Fix test", Description: ""},
			},
		}

		result := agent.BuildLogAnalysisPrompt(params)

		// Should contain the bead ID and title
		require.Contains(t, result, "beads-789")
		require.Contains(t, result, "Fix test")

		// Should not have extra whitespace issues
		require.Contains(t, result, "Existing Open Issues")
	})
}

func TestBeadSummaryStruct(t *testing.T) {
	summary := types.BeadSummary{
		ID:          "bead-test",
		Title:       "Test Title",
		Description: "Test Description",
	}

	require.Equal(t, "bead-test", summary.ID)
	require.Equal(t, "Test Title", summary.Title)
	require.Equal(t, "Test Description", summary.Description)
}

func TestNewAgent(t *testing.T) {
	tests := []struct {
		name        string
		cfg         *project.Config
		expectedBin string
	}{
		{
			name:        "Nil config defaults to claude",
			cfg:         nil,
			expectedBin: "claude",
		},
		{
			name:        "Empty type defaults to claude",
			cfg:         &project.Config{},
			expectedBin: "claude",
		},
		{
			name: "Claude type",
			cfg: &project.Config{
				Agent: project.AgentConfig{Type: "claude"},
			},
			expectedBin: "claude",
		},
		{
			name: "Pi type",
			cfg: &project.Config{
				Agent: project.AgentConfig{Type: "pi"},
			},
			expectedBin: "pi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent := NewAgent(tt.cfg)
			require.Equal(t, tt.expectedBin, agent.Binary())
		})
	}
}

func TestAgentBinary(t *testing.T) {
	require.Equal(t, "claude", NewAgent(nil).Binary())
	require.Equal(t, "pi", NewAgent(&project.Config{Agent: project.AgentConfig{Type: "pi"}}).Binary())
}

func TestBuildPlanPromptForPi(t *testing.T) {
	// Verify plan prompts are generated for both agent types
	claudeAgent := NewAgent(nil)
	piAgent := NewAgent(&project.Config{Agent: project.AgentConfig{Type: "pi"}})

	claudePrompt := claudeAgent.BuildPlanPrompt("bead-123")
	piPrompt := piAgent.BuildPlanPrompt("bead-123")

	require.Contains(t, claudePrompt, "bead-123")
	require.Contains(t, piPrompt, "bead-123")
	// Both should produce non-empty prompts
	require.NotEmpty(t, claudePrompt)
	require.NotEmpty(t, piPrompt)
}
