package agents

import (
	"strings"
	"testing"

	"github.com/sargehq/sarge/internal/agents/claude"
	"github.com/sargehq/sarge/internal/agents/pi"
	"github.com/sargehq/sarge/internal/agents/types"
	"github.com/sargehq/sarge/internal/project"
	"github.com/stretchr/testify/require"
)

// mustNewClaudeAgent is a test helper that creates a Claude agent for prompt testing.
func mustNewClaudeAgent(t *testing.T) *claude.Agent {
	t.Helper()
	return claude.New()
}

func TestBuildLogAnalysisPrompt(t *testing.T) {
	agent := mustNewClaudeAgent(t)

	tests := []struct {
		name           string
		params         types.TaskParams
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "Full parameters",
			params: types.TaskParams{
				Type:         types.TaskTypeLogAnalysis,
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
			params: types.TaskParams{
				Type:         types.TaskTypeLogAnalysis,
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
			params: types.TaskParams{
				Type:         types.TaskTypeLogAnalysis,
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
			params: types.TaskParams{
				Type:         types.TaskTypeLogAnalysis,
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
			result, err := agent.BuildPrompt(tt.params)
			require.NoError(t, err)

			for _, want := range tt.wantContains {
				require.Contains(t, result, want, "BuildPrompt() missing expected content")
			}

			for _, notWant := range tt.wantNotContain {
				require.NotContains(t, result, notWant, "BuildPrompt() contains unexpected content")
			}
		})
	}
}

func TestTaskParams(t *testing.T) {
	// Test that the struct can be properly initialized
	params := types.TaskParams{
		Type:         types.TaskTypeLogAnalysis,
		TaskID:       "task-1",
		WorkID:       "work-1",
		BranchName:   "main",
		RootIssueID:  "issue-1",
		WorkflowName: "workflow-1",
		JobName:      "job-1",
		LogFilePath:  "content",
	}

	require.Equal(t, types.TaskTypeLogAnalysis, params.Type)
	require.Equal(t, "task-1", params.TaskID)
	require.Equal(t, "work-1", params.WorkID)
	require.Equal(t, "main", params.BranchName)
	require.Equal(t, "issue-1", params.RootIssueID)
	require.Equal(t, "workflow-1", params.WorkflowName)
	require.Equal(t, "job-1", params.JobName)
	require.Equal(t, "content", params.LogFilePath)
}

func TestBuildLogAnalysisPromptPriorityGuidelines(t *testing.T) {
	agent := mustNewClaudeAgent(t)

	// Verify the prompt includes priority guidelines
	params := types.TaskParams{
		Type:         types.TaskTypeLogAnalysis,
		TaskID:       "w-test.1",
		WorkID:       "w-test",
		BranchName:   "main",
		WorkflowName: "CI",
		JobName:      "Test",
		LogFilePath:  "test failure",
	}

	result, err := agent.BuildPrompt(params)
	require.NoError(t, err)

	// Check that priority guidelines are included
	priorities := []string{
		"P0",
		"P1",
		"P2",
		"P3",
	}

	for _, p := range priorities {
		require.True(t, strings.Contains(result, p), "BuildPrompt() missing priority guideline: %s", p)
	}
}

func TestBuildLogAnalysisPromptBdCreateCommand(t *testing.T) {
	agent := mustNewClaudeAgent(t)

	// Verify the prompt includes bd create command examples
	params := types.TaskParams{
		Type:         types.TaskTypeLogAnalysis,
		TaskID:       "w-test.1",
		WorkID:       "w-test",
		BranchName:   "main",
		WorkflowName: "CI",
		JobName:      "Test",
		LogFilePath:  "test failure",
	}

	result, err := agent.BuildPrompt(params)
	require.NoError(t, err)

	// Check that bd create command format is included
	require.Contains(t, result, "bd create", "BuildPrompt() missing bd create command")

	// Check that it includes type options
	require.Contains(t, result, "--type", "BuildPrompt() missing --type flag")

	// Check that it includes priority option
	require.Contains(t, result, "--priority", "BuildPrompt() missing --priority flag")
}

func TestTaskParamsExistingBeadsField(t *testing.T) {
	// Test that ExistingBeads field is properly included in the struct
	params := types.TaskParams{
		Type:         types.TaskTypeLogAnalysis,
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
	agent := mustNewClaudeAgent(t)

	t.Run("renders existing beads section", func(t *testing.T) {
		params := types.TaskParams{
			Type:         types.TaskTypeLogAnalysis,
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

		result, err := agent.BuildPrompt(params)
		require.NoError(t, err)

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
		params := types.TaskParams{
			Type:          types.TaskTypeLogAnalysis,
			TaskID:        "w-test.1",
			WorkID:        "w-test",
			BranchName:    "main",
			WorkflowName:  "CI",
			JobName:       "Test",
			LogFilePath:   "--- FAIL: TestSomething (0.02s)",
			ExistingBeads: nil,
		}

		result, err := agent.BuildPrompt(params)
		require.NoError(t, err)

		// Should not contain the existing beads section header when no beads
		require.NotContains(t, result, "Existing Open Issues")
	})

	t.Run("handles beads without description", func(t *testing.T) {
		params := types.TaskParams{
			Type:         types.TaskTypeLogAnalysis,
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

		result, err := agent.BuildPrompt(params)
		require.NoError(t, err)

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
		wantPrompt  bool // can build a prompt without error
		wantType    string
	}{
		{
			name:       "Nil config defaults to claude",
			cfg:        nil,
			wantPrompt: true,
			wantType:   "*claude.Agent",
		},
		{
			name:       "Empty type defaults to claude",
			cfg:        &project.Config{},
			wantPrompt: true,
			wantType:   "*claude.Agent",
		},
		{
			name: "Claude type",
			cfg: &project.Config{
				Agent: project.AgentConfig{Type: "claude"},
			},
			wantPrompt: true,
			wantType:   "*claude.Agent",
		},
		{
			name: "Pi type",
			cfg: &project.Config{
				Agent: project.AgentConfig{Type: "pi"},
			},
			wantPrompt: true,
			wantType:   "*pi.Agent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			agent, err := NewAgent(tt.cfg)
			require.NoError(t, err)
			require.NotNil(t, agent)
			if tt.wantPrompt {
				// Use the concrete type to call BuildPrompt
				switch a := agent.(type) {
				case *claude.Agent:
					prompt, err := a.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeadID: "test-123"})
					require.NoError(t, err)
					require.NotEmpty(t, prompt)
					require.Contains(t, prompt, "test-123")
				case *pi.Agent:
					prompt, err := a.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeadID: "test-123"})
					require.NoError(t, err)
					require.NotEmpty(t, prompt)
					require.Contains(t, prompt, "test-123")
				default:
					t.Fatalf("unexpected agent type: %T", agent)
				}
			}
		})
	}
}

func TestBuildPlanPromptForPi(t *testing.T) {
	claudeAgent := claude.New()
	piAgent := pi.New()

	claudePrompt, err := claudeAgent.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeadID: "bead-123"})
	require.NoError(t, err)
	piPrompt, err := piAgent.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeadID: "bead-123"})
	require.NoError(t, err)

	require.Contains(t, claudePrompt, "bead-123")
	require.Contains(t, piPrompt, "bead-123")
	require.NotEmpty(t, claudePrompt)
	require.NotEmpty(t, piPrompt)
}

func TestNewAgentUnknownType(t *testing.T) {
	_, err := NewAgent(&project.Config{Agent: project.AgentConfig{Type: "unknown-agent"}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown agent type")
}

func TestBuildPromptUnknownType(t *testing.T) {
	agent := mustNewClaudeAgent(t)
	_, err := agent.BuildPrompt(types.TaskParams{Type: "nonexistent"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "unknown task type")
}

func TestBuildPromptAllTypes(t *testing.T) {
	agent := mustNewClaudeAgent(t)

	tests := []struct {
		name         string
		params       types.TaskParams
		wantContains string
	}{
		{
			name: "implement",
			params: types.TaskParams{
				Type:       types.TaskTypeImplement,
				TaskID:     "w-abc.1",
				BeadIDs:    []string{"bead-1"},
				BranchName: "feat/test",
				BaseBranch: "main",
			},
			wantContains: "w-abc.1",
		},
		{
			name: "estimate",
			params: types.TaskParams{
				Type:    types.TaskTypeEstimate,
				TaskID:  "w-abc.2",
				BeadIDs: []string{"bead-1"},
			},
			wantContains: "w-abc.2",
		},
		{
			name: "pr",
			params: types.TaskParams{
				Type:       types.TaskTypePR,
				TaskID:     "w-abc.3",
				WorkID:     "w-abc",
				BranchName: "feat/test",
				BaseBranch: "main",
			},
			wantContains: "w-abc",
		},
		{
			name: "review",
			params: types.TaskParams{
				Type:        types.TaskTypeReview,
				TaskID:      "w-abc.4",
				WorkID:      "w-abc",
				BranchName:  "feat/test",
				BaseBranch:  "main",
				RootIssueID: "root-1",
			},
			wantContains: "w-abc",
		},
		{
			name: "update_pr_description",
			params: types.TaskParams{
				Type:       types.TaskTypeUpdatePRDescription,
				TaskID:     "w-abc.5",
				WorkID:     "w-abc",
				PRURL:      "https://github.com/test/pr/1",
				BranchName: "feat/test",
				BaseBranch: "main",
			},
			wantContains: "https://github.com/test/pr/1",
		},
		{
			name: "plan",
			params: types.TaskParams{
				Type:   types.TaskTypePlan,
				BeadID: "bead-plan-1",
			},
			wantContains: "bead-plan-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := agent.BuildPrompt(tt.params)
			require.NoError(t, err)
			require.NotEmpty(t, result)
			require.Contains(t, result, tt.wantContains)
		})
	}
}
