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
				RootIssueID:  "beans-123",
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
				"--parent beans-123",
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
				RootIssueID:  "beans-456",
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

	// Verify the prompt includes beans create command examples
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

	// Check that beans create command format is included
	require.Contains(t, result, "beans create", "BuildPrompt() missing beans create command")

	// Check that it includes type options
	require.Contains(t, result, "--type", "BuildPrompt() missing --type flag")

	// Check that it includes priority option
	require.Contains(t, result, "--priority", "BuildPrompt() missing --priority flag")
}

func TestTaskParamsExistingBeansField(t *testing.T) {
	// Test that ExistingBeans field is properly included in the struct
	params := types.TaskParams{
		Type:         types.TaskTypeLogAnalysis,
		TaskID:       "task-1",
		WorkID:       "work-1",
		BranchName:   "main",
		RootIssueID:  "issue-1",
		WorkflowName: "workflow-1",
		JobName:      "job-1",
		LogFilePath:  "content",
		ExistingBeans: []types.BeanSummary{
			{ID: "bean-1", Title: "Fix test failure", Body: "Test failed at file.go:42"},
			{ID: "bean-2", Title: "Lint error", Body: "Missing comment on exported function"},
		},
	}

	require.Len(t, params.ExistingBeans, 2)
	require.Equal(t, "bean-1", params.ExistingBeans[0].ID)
	require.Equal(t, "Fix test failure", params.ExistingBeans[0].Title)
	require.Equal(t, "Test failed at file.go:42", params.ExistingBeans[0].Body)
}

func TestBuildLogAnalysisPromptWithExistingBeans(t *testing.T) {
	agent := mustNewClaudeAgent(t)

	t.Run("renders existing beans section", func(t *testing.T) {
		params := types.TaskParams{
			Type:         types.TaskTypeLogAnalysis,
			TaskID:       "w-test.1",
			WorkID:       "w-test",
			BranchName:   "main",
			WorkflowName: "CI",
			JobName:      "Test",
			LogFilePath:  "--- FAIL: TestSomething (0.02s)",
			ExistingBeans: []types.BeanSummary{
				{ID: "beans-123", Title: "Fix TestUserAuth failure", Body: "Test failed at auth_test.go:42"},
				{ID: "beans-456", Title: "Fix lint error in utils", Body: "Missing comment"},
			},
		}

		result, err := agent.BuildPrompt(params)
		require.NoError(t, err)

		// Should contain the existing beans section header
		require.Contains(t, result, "Existing Open Issues")

		// Should contain bean IDs and titles
		require.Contains(t, result, "beans-123")
		require.Contains(t, result, "Fix TestUserAuth failure")
		require.Contains(t, result, "beans-456")
		require.Contains(t, result, "Fix lint error in utils")

		// Should contain descriptions
		require.Contains(t, result, "Test failed at auth_test.go:42")
		require.Contains(t, result, "Missing comment")

		// Should contain instructions to check existing beans
		require.Contains(t, result, "matches an existing issue")
		require.Contains(t, result, "skip it")
	})

	t.Run("no existing beans section when empty", func(t *testing.T) {
		params := types.TaskParams{
			Type:          types.TaskTypeLogAnalysis,
			TaskID:        "w-test.1",
			WorkID:        "w-test",
			BranchName:    "main",
			WorkflowName:  "CI",
			JobName:       "Test",
			LogFilePath:   "--- FAIL: TestSomething (0.02s)",
			ExistingBeans: nil,
		}

		result, err := agent.BuildPrompt(params)
		require.NoError(t, err)

		// Should not contain the existing beans section header when no beans
		require.NotContains(t, result, "Existing Open Issues")
	})

	t.Run("handles beans without description", func(t *testing.T) {
		params := types.TaskParams{
			Type:         types.TaskTypeLogAnalysis,
			TaskID:       "w-test.1",
			WorkID:       "w-test",
			BranchName:   "main",
			WorkflowName: "CI",
			JobName:      "Test",
			LogFilePath:  "--- FAIL: TestSomething (0.02s)",
			ExistingBeans: []types.BeanSummary{
				{ID: "beans-789", Title: "Fix test", Body: ""},
			},
		}

		result, err := agent.BuildPrompt(params)
		require.NoError(t, err)

		// Should contain the bean ID and title
		require.Contains(t, result, "beans-789")
		require.Contains(t, result, "Fix test")

		// Should not have extra whitespace issues
		require.Contains(t, result, "Existing Open Issues")
	})
}

func TestBeanSummaryStruct(t *testing.T) {
	summary := types.BeanSummary{
		ID:          "bean-test",
		Title:       "Test Title",
		Body: "Test Description",
	}

	require.Equal(t, "bean-test", summary.ID)
	require.Equal(t, "Test Title", summary.Title)
	require.Equal(t, "Test Description", summary.Body)
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
					prompt, err := a.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeanID: "test-123"})
					require.NoError(t, err)
					require.NotEmpty(t, prompt)
					require.Contains(t, prompt, "test-123")
				case *pi.Agent:
					prompt, err := a.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeanID: "test-123"})
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

	claudePrompt, err := claudeAgent.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeanID: "bean-123"})
	require.NoError(t, err)
	piPrompt, err := piAgent.BuildPrompt(types.TaskParams{Type: types.TaskTypePlan, BeanID: "bean-123"})
	require.NoError(t, err)

	require.Contains(t, claudePrompt, "bean-123")
	require.Contains(t, piPrompt, "bean-123")
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
				BeanIDs:    []string{"bean-1"},
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
				BeanIDs: []string{"bean-1"},
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
				BeanID: "bean-plan-1",
			},
			wantContains: "bean-plan-1",
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
