package feedback

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/feedback/logparser"
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
		{"Sarge resolution comment", "<!-- sarge-bot -->✅ Resolved in work w-abc (issue ac-123)", false},
		{"Sarge ack comment", "<!-- sarge-bot -->✅ Created tracking issue **ac-456** for this feedback.\n\nTitle: Fix bug\nPriority: P2", false},
		{"Sarge marker with actionable text", "<!-- sarge-bot -->Please fix this issue", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.isActionableComment(tt.body)
			require.Equal(t, tt.actionable, result)
		})
	}
}

func TestProcessStatusChecks(t *testing.T) {
	processor := &FeedbackProcessor{
		client: &github.GitHubClientMock{},
	}

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

	items := processor.processStatusChecks(context.Background(), "owner/repo", status)

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

func TestNormalizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "strips ISO timestamp",
			input:    "Error at 2024-01-15 10:30:45: connection failed",
			expected: "Error at : connection failed",
		},
		{
			name:     "strips ISO timestamp with T separator",
			input:    "Error at 2024-01-15T10:30:45Z: connection failed",
			expected: "Error at : connection failed",
		},
		{
			name:     "strips timestamp with timezone offset",
			input:    "Error at 2024-01-15T10:30:45+00:00: connection failed",
			expected: "Error at : connection failed",
		},
		{
			name:     "strips short time",
			input:    "10:30:45 Error: test failed",
			expected: "Error: test failed",
		},
		{
			name:     "strips memory address",
			input:    "panic at 0x7fff5fbff8c0: invalid pointer",
			expected: "panic at : invalid pointer",
		},
		{
			name:     "strips multiple memory addresses",
			input:    "value 0xDEADBEEF != expected 0xCAFEBABE",
			expected: "value != expected",
		},
		{
			name:     "strips temp path /tmp",
			input:    "failed to write /tmp/test-123/output.txt",
			expected: "failed to write",
		},
		{
			name:     "strips temp path /var/folders",
			input:    "failed to read /var/folders/ab/cd/T/go-build123/test.go",
			expected: "failed to read",
		},
		{
			name:     "strips pid= format",
			input:    "process pid=12345 crashed",
			expected: "process crashed",
		},
		{
			name:     "strips PID: format",
			input:    "Process PID: 67890 terminated",
			expected: "Process terminated",
		},
		{
			name:     "strips goroutine ID",
			input:    "goroutine 123 [running]:\nmain.doSomething()",
			expected: "[running]: main.doSomething()",
		},
		{
			name:     "strips duration values",
			input:    "--- FAIL: TestSomething (1.234s)",
			expected: "--- FAIL: TestSomething",
		},
		{
			name:     "strips external stack trace lines",
			input:    "called from /usr/local/go/src/runtime/panic.go:123",
			expected: "called from",
		},
		{
			name:     "collapses whitespace",
			input:    "error   with    multiple   spaces",
			expected: "error with multiple spaces",
		},
		{
			name:     "preserves error message core",
			input:    "TestUserAuth: expected valid token, got expired",
			expected: "TestUserAuth: expected valid token, got expired",
		},
		{
			name:     "handles complex CI log snippet",
			input:    "2024-01-15 10:30:45 goroutine 42 [running]: TestAuth failed at 0x7fff5fbff8c0 (1.234s)",
			expected: "[running]: TestAuth failed at",
		},
		{
			name:     "empty input",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeContent(tt.input)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestGenerateFailureSourceID(t *testing.T) {
	t.Run("same logical failure produces same hash", func(t *testing.T) {
		// Simulate same test failure from different CI runs
		id1 := generateFailureSourceID(
			"TestUserAuth",
			"auth_test.go",
			42,
			"2024-01-15 10:30:45 Error: expected valid token, got expired",
		)
		id2 := generateFailureSourceID(
			"TestUserAuth",
			"auth_test.go",
			42,
			"2024-01-16 14:22:33 Error: expected valid token, got expired",
		)

		require.Equal(t, id1, id2, "Same logical failure should produce same ID regardless of timestamp")
		require.True(t, len(id1) > 0, "ID should not be empty")
		require.Contains(t, id1, "fail-", "ID should have fail- prefix")
	})

	t.Run("different failures produce different hashes", func(t *testing.T) {
		id1 := generateFailureSourceID(
			"TestUserAuth",
			"auth_test.go",
			42,
			"Error: expected valid token, got expired",
		)
		id2 := generateFailureSourceID(
			"TestUserCreate",
			"user_test.go",
			100,
			"Error: user already exists",
		)

		require.NotEqual(t, id1, id2, "Different failures should produce different IDs")
	})

	t.Run("different line numbers produce different hashes", func(t *testing.T) {
		id1 := generateFailureSourceID("TestSomething", "test.go", 42, "error msg")
		id2 := generateFailureSourceID("TestSomething", "test.go", 43, "error msg")

		require.NotEqual(t, id1, id2, "Different line numbers should produce different IDs")
	})

	t.Run("normalizes memory addresses in message", func(t *testing.T) {
		id1 := generateFailureSourceID(
			"TestPointer",
			"ptr_test.go",
			10,
			"invalid pointer at 0xDEADBEEF",
		)
		id2 := generateFailureSourceID(
			"TestPointer",
			"ptr_test.go",
			10,
			"invalid pointer at 0xCAFEBABE",
		)

		require.Equal(t, id1, id2, "Memory addresses should be normalized")
	})

	t.Run("normalizes PIDs in message", func(t *testing.T) {
		id1 := generateFailureSourceID(
			"TestProcess",
			"proc_test.go",
			20,
			"process pid=12345 crashed",
		)
		id2 := generateFailureSourceID(
			"TestProcess",
			"proc_test.go",
			20,
			"process pid=67890 crashed",
		)

		require.Equal(t, id1, id2, "PIDs should be normalized")
	})
}

func TestGenerateGenericFailureSourceID(t *testing.T) {
	t.Run("same workflow failure produces same hash", func(t *testing.T) {
		id1 := generateGenericFailureSourceID("CI Pipeline", "Build", "Compile")
		id2 := generateGenericFailureSourceID("CI Pipeline", "Build", "Compile")

		require.Equal(t, id1, id2, "Same workflow failure should produce same ID")
		require.Contains(t, id1, "fail-", "ID should have fail- prefix")
	})

	t.Run("different workflow failures produce different hashes", func(t *testing.T) {
		id1 := generateGenericFailureSourceID("CI Pipeline", "Build", "Compile")
		id2 := generateGenericFailureSourceID("CI Pipeline", "Test", "Run tests")

		require.NotEqual(t, id1, id2, "Different workflow failures should produce different IDs")
	})

	t.Run("different workflow names produce different hashes", func(t *testing.T) {
		id1 := generateGenericFailureSourceID("CI Pipeline", "Build", "Compile")
		id2 := generateGenericFailureSourceID("Nightly Build", "Build", "Compile")

		require.NotEqual(t, id1, id2, "Different workflow names should produce different IDs")
	})
}

func TestSourceIDStabilityAcrossCIRuns(t *testing.T) {
	// Simulate feedback items from the same failure across different CI runs
	t.Run("createFailureItem produces stable IDs", func(t *testing.T) {
		processor := &FeedbackProcessor{
			client: &github.Client{},
		}

		// First CI run
		workflow1 := github.WorkflowRun{
			ID:   12345, // Different run ID
			Name: "Test Suite",
		}
		job1 := github.Job{
			ID:   111111, // Different job ID
			Name: "Unit Tests",
		}
		failure1 := logparser.Failure{
			Name:    "TestUserAuth",
			File:    "auth_test.go",
			Line:    42,
			Message: "2024-01-15 10:30:45 Error: expected valid token",
		}

		// Second CI run with different IDs but same failure
		workflow2 := github.WorkflowRun{
			ID:   67890, // Different run ID
			Name: "Test Suite",
		}
		job2 := github.Job{
			ID:   222222, // Different job ID
			Name: "Unit Tests",
		}
		failure2 := logparser.Failure{
			Name:    "TestUserAuth",
			File:    "auth_test.go",
			Line:    42,
			Message: "2024-01-16 14:22:33 Error: expected valid token",
		}

		item1 := processor.createFailureItem(workflow1, job1, failure1)
		item2 := processor.createFailureItem(workflow2, job2, failure2)

		require.Equal(t, item1.Source.ID, item2.Source.ID,
			"Same logical failure should have same source ID across CI runs")
	})

	t.Run("createGenericFailureItem produces stable IDs", func(t *testing.T) {
		processor := &FeedbackProcessor{
			client: &github.Client{},
		}

		// First CI run
		workflow1 := github.WorkflowRun{
			ID:   12345,
			Name: "CI Pipeline",
		}
		job1 := github.Job{
			ID:         111111,
			Name:       "Build",
			Conclusion: "failure",
			Steps: []github.Step{
				{Name: "Setup", Conclusion: "success"},
				{Name: "Compile", Conclusion: "failure"},
			},
		}

		// Second CI run
		workflow2 := github.WorkflowRun{
			ID:   67890,
			Name: "CI Pipeline",
		}
		job2 := github.Job{
			ID:         222222,
			Name:       "Build",
			Conclusion: "failure",
			Steps: []github.Step{
				{Name: "Setup", Conclusion: "success"},
				{Name: "Compile", Conclusion: "failure"},
			},
		}

		item1 := processor.createGenericFailureItem(workflow1, job1)
		item2 := processor.createGenericFailureItem(workflow2, job2)

		require.Equal(t, item1.Source.ID, item2.Source.ID,
			"Same workflow failure should have same source ID across CI runs")
	})
}

func TestParseGitHubActionsURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantRepo  string
		wantRunID int64
		wantJobID int64
		wantOk    bool
	}{
		{
			name:      "valid GitHub Actions URL",
			url:       "https://github.com/sargehq/sarge/actions/runs/21783874747/job/62852104893",
			wantRepo:  "sargehq/sarge",
			wantRunID: 21783874747,
			wantJobID: 62852104893,
			wantOk:    true,
		},
		{
			name:   "non-GitHub Actions URL",
			url:    "https://example.com/checks/1",
			wantOk: false,
		},
		{
			name:   "empty URL",
			url:    "",
			wantOk: false,
		},
		{
			name:   "GitHub URL but not actions",
			url:    "https://github.com/sargehq/sarge/pull/22",
			wantOk: false,
		},
		{
			name:   "GitHub Actions run URL without job",
			url:    "https://github.com/sargehq/sarge/actions/runs/21783874747",
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, runID, jobID, ok := parseGitHubActionsURL(tt.url)
			require.Equal(t, tt.wantOk, ok)
			if ok {
				require.Equal(t, tt.wantRepo, repo)
				require.Equal(t, tt.wantRunID, runID)
				require.Equal(t, tt.wantJobID, jobID)
			}
		})
	}
}

func TestHasMatchingWorkflowRun(t *testing.T) {
	processor := &FeedbackProcessor{}

	workflows := []github.WorkflowRun{
		{
			Name: "CI",
			Jobs: []github.Job{
				{Name: "lint"},
				{Name: "test"},
			},
		},
		{
			Name: "Build",
			Jobs: []github.Job{
				{Name: "compile"},
			},
		},
	}

	tests := []struct {
		name      string
		checkName string
		want      bool
	}{
		{"matches workflow name", "CI", true},
		{"matches workflow name case-insensitive", "ci", true},
		{"matches job name", "lint", true},
		{"matches job name case-insensitive", "Lint", true},
		{"matches another job", "test", true},
		{"matches another workflow", "Build", true},
		{"no match", "security-scan", false},
		{"partial match is not a match", "lin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := processor.hasMatchingWorkflowRun(tt.checkName, workflows)
			require.Equal(t, tt.want, result)
		})
	}
}

func TestProcessStatusChecks_DeduplicatesAgainstWorkflowRuns(t *testing.T) {
	processor := &FeedbackProcessor{
		client: &github.GitHubClientMock{},
	}

	status := &github.PRStatus{
		StatusChecks: []github.StatusCheck{
			{
				Context: "lint",
				State:   "FAILURE",
			},
			{
				Context: "security-scan",
				State:   "FAILURE",
			},
		},
		Workflows: []github.WorkflowRun{
			{
				Name:       "CI",
				Conclusion: "failure",
				Jobs: []github.Job{
					{Name: "lint", Conclusion: "failure"},
				},
			},
		},
	}

	items := processor.processStatusChecks(context.Background(), "owner/repo", status)

	// Only security-scan should appear; lint is deduplicated against the workflow run
	require.Len(t, items, 1)
	require.Equal(t, "Fix security-scan failure", items[0].Title)
}

func TestProcessStatusChecks_EnrichesFromGitHubActionsURL(t *testing.T) {
	mockClient := &github.GitHubClientMock{
		GetJobLogsFunc: func(ctx context.Context, repo string, jobID int64) (string, error) {
			require.Equal(t, "sargehq/sarge", repo)
			require.Equal(t, int64(62852104893), jobID)
			// Return logs that the parser can extract failures from
			return "=== RUN   TestFoo\n--- FAIL: TestFoo (0.00s)\n    foo_test.go:10: expected true, got false\nFAIL\n", nil
		},
	}

	processor := &FeedbackProcessor{
		client: mockClient,
	}

	status := &github.PRStatus{
		StatusChecks: []github.StatusCheck{
			{
				Context:   "unit-tests",
				State:     "FAILURE",
				TargetURL: "https://github.com/sargehq/sarge/actions/runs/21783874747/job/62852104893",
			},
		},
	}

	items := processor.processStatusChecks(context.Background(), "sargehq/sarge", status)

	// Should have enriched items from parsed logs
	require.Greater(t, len(items), 0)
	// The items should have detailed info, not just "Fix unit-tests failure"
	for _, item := range items {
		require.NotEqual(t, "Fix unit-tests failure", item.Title, "Should have enriched title from log parsing")
		require.Equal(t, github.SourceTypeCI, item.Source.Type)
		require.Equal(t, "unit-tests", item.Source.Name)
	}
}

func TestProcessStatusChecks_FallsBackForNonGHActionsURL(t *testing.T) {
	processor := &FeedbackProcessor{
		client: &github.GitHubClientMock{},
	}

	status := &github.PRStatus{
		StatusChecks: []github.StatusCheck{
			{
				Context:     "external-ci",
				State:       "FAILURE",
				Description: "External CI failed",
				TargetURL:   "https://jenkins.example.com/job/123",
			},
		},
	}

	items := processor.processStatusChecks(context.Background(), "owner/repo", status)

	require.Len(t, items, 1)
	require.Equal(t, "Fix external-ci failure", items[0].Title)
	require.Equal(t, "External CI failed", items[0].Description)
}

func TestProcessStatusChecks_FallsBackForNoTargetURL(t *testing.T) {
	processor := &FeedbackProcessor{
		client: &github.GitHubClientMock{},
	}

	status := &github.PRStatus{
		StatusChecks: []github.StatusCheck{
			{
				Context: "some-check",
				State:   "FAILURE",
			},
		},
	}

	items := processor.processStatusChecks(context.Background(), "owner/repo", status)

	require.Len(t, items, 1)
	require.Equal(t, "Fix some-check failure", items[0].Title)
	require.Contains(t, items[0].Description, "CI check 'some-check' failed with state: FAILURE")
}
