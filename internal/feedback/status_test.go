package feedback

import (
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/stretchr/testify/require"
)

func TestExtractCIStatus(t *testing.T) {
	// Note: extractCIStatus only uses StatusChecks (from `gh pr checks`),
	// not Workflows. Workflow runs are only used for extracting detailed
	// failure information, not for determining overall CI status.
	tests := []struct {
		name     string
		status   *github.PRStatus
		expected string
	}{
		{
			name:     "no checks returns pending",
			status:   &github.PRStatus{},
			expected: db.CIStatusPending,
		},
		{
			name: "all status checks success",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "test1", State: "SUCCESS"},
					{Context: "test2", State: "SUCCESS"},
					{Context: "test3", State: "SKIPPED"},
				},
			},
			expected: db.CIStatusSuccess,
		},
		{
			name: "one status check failure",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "test1", State: "SUCCESS"},
					{Context: "test2", State: "FAILURE"},
				},
			},
			expected: db.CIStatusFailure,
		},
		{
			name: "one status check pending",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "test1", State: "SUCCESS"},
					{Context: "test2", State: "PENDING"},
				},
			},
			expected: db.CIStatusPending,
		},
		{
			name: "status check with empty state is pending",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "test1", State: "SUCCESS"},
					{Context: "test2", State: ""},
				},
			},
			expected: db.CIStatusPending,
		},
		{
			name: "status check error",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "test1", State: "ERROR"},
				},
			},
			expected: db.CIStatusFailure,
		},
		{
			name: "workflows are ignored for CI status - only checks matter",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "Build", State: "SUCCESS"},
				},
				// Workflows are present but should be ignored
				Workflows: []github.WorkflowRun{
					{Name: "CI", Status: "completed", Conclusion: "failure"},
				},
			},
			expected: db.CIStatusSuccess,
		},
		{
			name: "multiple checks all success",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "Build", State: "SUCCESS"},
					{Context: "Test", State: "SUCCESS"},
					{Context: "Lint", State: "SUCCESS"},
				},
			},
			expected: db.CIStatusSuccess,
		},
		{
			name: "failure takes precedence over pending",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "Build", State: "FAILURE"},
					{Context: "Test", State: "PENDING"},
				},
			},
			expected: db.CIStatusFailure,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractCIStatus(tt.status)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractApprovalStatus(t *testing.T) {
	now := time.Now()
	earlier := now.Add(-1 * time.Hour)

	tests := []struct {
		name              string
		status            *github.PRStatus
		expectedStatus    string
		expectedApprovers []string
	}{
		{
			name:              "no reviews",
			status:            &github.PRStatus{},
			expectedStatus:    db.ApprovalStatusPending,
			expectedApprovers: []string{},
		},
		{
			name: "one approval",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "user1", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusApproved,
			expectedApprovers: []string{"user1"},
		},
		{
			name: "multiple approvals",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "user1", CreatedAt: now},
					{ID: 2, State: "APPROVED", Author: "user2", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusApproved,
			expectedApprovers: []string{"user1", "user2"},
		},
		{
			name: "changes requested",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "CHANGES_REQUESTED", Author: "user1", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusChangesRequested,
			expectedApprovers: []string{},
		},
		{
			name: "changes requested takes precedence over approval",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "user1", CreatedAt: now},
					{ID: 2, State: "CHANGES_REQUESTED", Author: "user2", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusChangesRequested,
			expectedApprovers: []string{"user1"},
		},
		{
			name: "commented reviews are ignored",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "COMMENTED", Author: "user1", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusPending,
			expectedApprovers: []string{},
		},
		{
			name: "later approval overrides earlier changes requested",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "CHANGES_REQUESTED", Author: "user1", CreatedAt: earlier},
					{ID: 2, State: "APPROVED", Author: "user1", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusApproved,
			expectedApprovers: []string{"user1"},
		},
		{
			name: "later changes requested overrides earlier approval",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "user1", CreatedAt: earlier},
					{ID: 2, State: "CHANGES_REQUESTED", Author: "user1", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusChangesRequested,
			expectedApprovers: []string{},
		},
		{
			name: "bot approval counts",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "github-actions[bot]", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusApproved,
			expectedApprovers: []string{"github-actions[bot]"},
		},
		{
			name: "mixed commented and approved",
			status: &github.PRStatus{
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "user1", CreatedAt: now},
					{ID: 2, State: "COMMENTED", Author: "user2", CreatedAt: now},
					{ID: 3, State: "COMMENTED", Author: "user3", CreatedAt: now},
				},
			},
			expectedStatus:    db.ApprovalStatusApproved,
			expectedApprovers: []string{"user1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, approvers := extractApprovalStatus(tt.status)
			require.Equal(t, tt.expectedStatus, status)

			// Check approvers (order may vary)
			require.Len(t, approvers, len(tt.expectedApprovers))

			if len(approvers) > 0 {
				approverSet := make(map[string]bool)
				for _, a := range approvers {
					approverSet[a] = true
				}
				for _, expected := range tt.expectedApprovers {
					require.True(t, approverSet[expected], "missing approver %q", expected)
				}
			}
		})
	}
}

func TestExtractStatusFromPRStatus(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name              string
		status            *github.PRStatus
		expectedCI        string
		expectedApproval  string
		expectedApprovers []string
	}{
		{
			name:              "empty status",
			status:            &github.PRStatus{},
			expectedCI:        db.CIStatusPending,
			expectedApproval:  db.ApprovalStatusPending,
			expectedApprovers: []string{},
		},
		{
			name: "all passing and approved",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "CI", State: "SUCCESS"},
				},
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "reviewer", CreatedAt: now},
				},
			},
			expectedCI:        db.CIStatusSuccess,
			expectedApproval:  db.ApprovalStatusApproved,
			expectedApprovers: []string{"reviewer"},
		},
		{
			name: "ci failing but approved",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "CI", State: "FAILURE"},
				},
				Reviews: []github.Review{
					{ID: 1, State: "APPROVED", Author: "reviewer", CreatedAt: now},
				},
			},
			expectedCI:        db.CIStatusFailure,
			expectedApproval:  db.ApprovalStatusApproved,
			expectedApprovers: []string{"reviewer"},
		},
		{
			name: "ci passing but changes requested",
			status: &github.PRStatus{
				StatusChecks: []github.StatusCheck{
					{Context: "CI", State: "SUCCESS"},
				},
				Reviews: []github.Review{
					{ID: 1, State: "CHANGES_REQUESTED", Author: "reviewer", CreatedAt: now},
				},
			},
			expectedCI:        db.CIStatusSuccess,
			expectedApproval:  db.ApprovalStatusChangesRequested,
			expectedApprovers: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := ExtractStatusFromPRStatus(tt.status)

			require.Equal(t, tt.expectedCI, info.CIStatus)
			require.Equal(t, tt.expectedApproval, info.ApprovalStatus)
			require.Len(t, info.Approvers, len(tt.expectedApprovers))
		})
	}
}
