package feedback

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
)

// PRStatusInfo represents the extracted PR status information.
type PRStatusInfo struct {
	CIStatus       string   // pending, success, failure
	ApprovalStatus string   // pending, approved, changes_requested
	Approvers      []string // List of usernames who approved
	PRState        string   // open, closed, merged
	MergeableState string   // CLEAN, DIRTY, BLOCKED, BEHIND, DRAFT, UNSTABLE, UNKNOWN
}

// ExtractStatusFromPRStatus extracts CI and approval status from a PRStatus object.
func ExtractStatusFromPRStatus(status *github.PRStatus) *PRStatusInfo {
	info := &PRStatusInfo{
		CIStatus:       db.CIStatusPending,
		ApprovalStatus: db.ApprovalStatusPending,
		Approvers:      []string{},
		PRState:        normalizePRState(status.State),
		MergeableState: status.MergeableState,
	}

	// Extract CI status from status checks and workflow runs
	info.CIStatus = extractCIStatus(status)

	// Extract approval status and approvers from reviews
	info.ApprovalStatus, info.Approvers = extractApprovalStatus(status)

	return info
}

// normalizePRState converts GitHub PR state to our normalized state.
// GitHub uses: OPEN, CLOSED, MERGED (uppercase)
// We use: open, closed, merged (lowercase)
func normalizePRState(state string) string {
	switch strings.ToUpper(state) {
	case "OPEN":
		return db.PRStateOpen
	case "CLOSED":
		return db.PRStateClosed
	case "MERGED":
		return db.PRStateMerged
	default:
		return db.PRStateOpen // Default to open if unknown
	}
}

// extractCIStatus determines the overall CI status from status checks.
// Returns: db.CIStatusPending, db.CIStatusSuccess, or db.CIStatusFailure
//
// We only use StatusChecks (from `gh pr checks`) for CI status determination.
// StatusChecks already represent the current aggregated state of all checks
// for the PR's head commit. Workflow runs are only used separately for
// extracting detailed failure information (job logs, test names, etc.).
func extractCIStatus(status *github.PRStatus) string {
	if len(status.StatusChecks) == 0 {
		// No CI configured
		return db.CIStatusPending
	}

	// Check for any failures
	for _, check := range status.StatusChecks {
		if check.State == "FAILURE" || check.State == "ERROR" {
			return db.CIStatusFailure
		}
	}

	// Check for any pending
	for _, check := range status.StatusChecks {
		if check.State == "PENDING" || check.State == "" {
			return db.CIStatusPending
		}
	}

	// All checks passed
	return db.CIStatusSuccess
}

// extractApprovalStatus determines the approval status from reviews.
// Returns: (status, approvers) where status is db.ApprovalStatusPending, db.ApprovalStatusApproved, or db.ApprovalStatusChangesRequested
func extractApprovalStatus(status *github.PRStatus) (string, []string) {
	if len(status.Reviews) == 0 {
		return db.ApprovalStatusPending, []string{}
	}

	// Track the latest review state per user
	// Later reviews override earlier ones for the same user
	latestStateByUser := make(map[string]string)
	latestTimeByUser := make(map[string]time.Time)

	for _, review := range status.Reviews {
		// Skip COMMENTED reviews - they don't affect approval status
		if review.State == "COMMENTED" {
			continue
		}

		// Only update if this review is newer than the previous one from this user
		if prevTime, exists := latestTimeByUser[review.Author]; !exists || review.CreatedAt.After(prevTime) {
			latestStateByUser[review.Author] = review.State
			latestTimeByUser[review.Author] = review.CreatedAt
		}
	}

	// Collect approvers and check for changes requested
	var approvers []string
	hasChangesRequested := false

	for user, state := range latestStateByUser {
		switch state {
		case "APPROVED":
			approvers = append(approvers, user)
		case "CHANGES_REQUESTED":
			hasChangesRequested = true
		}
	}

	// Determine overall status
	// If any reviewer has requested changes, the status is "changes_requested"
	// If at least one reviewer has approved (and no changes requested), status is "approved"
	// Otherwise, status is "pending"
	if hasChangesRequested {
		return db.ApprovalStatusChangesRequested, approvers
	}
	if len(approvers) > 0 {
		return db.ApprovalStatusApproved, approvers
	}
	return db.ApprovalStatusPending, []string{}
}

// ApproversToJSON converts a list of approvers to a JSON string.
func ApproversToJSON(approvers []string) string {
	if len(approvers) == 0 {
		return "[]"
	}
	data, err := json.Marshal(approvers)
	if err != nil {
		return "[]"
	}
	return string(data)
}

// ApproversFromJSON parses a JSON string into a list of approvers.
func ApproversFromJSON(jsonStr string) []string {
	if jsonStr == "" || jsonStr == "[]" {
		return []string{}
	}
	var approvers []string
	if err := json.Unmarshal([]byte(jsonStr), &approvers); err != nil {
		return []string{}
	}
	return approvers
}

// UpdatePRStatusIfChanged compares the new PR status with the stored status
// and updates the database if anything changed. Returns true if status changed.
func UpdatePRStatusIfChanged(ctx context.Context, database *db.DB, work *db.Work, newStatus *PRStatusInfo, quiet bool) bool {
	// Get current approvers from work (stored as JSON)
	currentApprovers := ApproversFromJSON(work.Approvers)

	// Check if anything changed
	ciChanged := work.CIStatus != newStatus.CIStatus
	approvalChanged := work.ApprovalStatus != newStatus.ApprovalStatus
	approversChanged := !stringSlicesEqual(currentApprovers, newStatus.Approvers)
	prStateChanged := work.PRState != newStatus.PRState
	mergeableChanged := work.MergeableState != newStatus.MergeableState

	if !ciChanged && !approvalChanged && !approversChanged && !prStateChanged && !mergeableChanged {
		// No changes
		if !quiet {
			fmt.Printf("PR status unchanged: CI=%s, Approval=%s, State=%s, Mergeable=%s\n", work.CIStatus, work.ApprovalStatus, work.PRState, work.MergeableState)
		}
		return false
	}

	// Log what changed
	if !quiet {
		if ciChanged {
			fmt.Printf("CI status changed: %s -> %s\n", work.CIStatus, newStatus.CIStatus)
		}
		if approvalChanged {
			fmt.Printf("Approval status changed: %s -> %s\n", work.ApprovalStatus, newStatus.ApprovalStatus)
		}
		if approversChanged {
			fmt.Printf("Approvers changed: %v -> %v\n", currentApprovers, newStatus.Approvers)
		}
		if prStateChanged {
			fmt.Printf("PR state changed: %s -> %s\n", work.PRState, newStatus.PRState)
		}
		if mergeableChanged {
			fmt.Printf("Mergeable state changed: %s -> %s\n", work.MergeableState, newStatus.MergeableState)
		}
	}

	// Convert approvers to JSON
	approversJSON := ApproversToJSON(newStatus.Approvers)

	// Update the database
	if err := database.UpdateWorkPRStatus(ctx, work.ID, newStatus.CIStatus, newStatus.ApprovalStatus, approversJSON, newStatus.PRState, newStatus.MergeableState); err != nil {
		if !quiet {
			fmt.Printf("Warning: failed to update PR status: %v\n", err)
		}
		return false
	}

	// If PR was merged, transition work to merged status
	if newStatus.PRState == db.PRStateMerged && work.Status != db.StatusMerged {
		if !quiet {
			fmt.Printf("PR merged! Transitioning work %s to merged status\n", work.ID)
		}
		if err := database.MergeWork(ctx, work.ID); err != nil {
			if !quiet {
				fmt.Printf("Warning: failed to mark work as merged: %v\n", err)
			}
		}
	}

	// Mark as having unseen changes
	if err := database.SetWorkHasUnseenPRChanges(ctx, work.ID, true); err != nil {
		if !quiet {
			fmt.Printf("Warning: failed to set unseen PR changes: %v\n", err)
		}
	}

	return true
}

// stringSlicesEqual compares two string slices for equality (order-independent)
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	// Create maps for comparison
	aMap := make(map[string]int)
	for _, s := range a {
		aMap[s]++
	}

	for _, s := range b {
		if aMap[s] == 0 {
			return false
		}
		aMap[s]--
	}

	return true
}
