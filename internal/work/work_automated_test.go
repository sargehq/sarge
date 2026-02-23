package work

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/stretchr/testify/assert"
)

func TestGenerateBranchNameFromBead_BasicTitle(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add user authentication",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBead_UppercaseTitle(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "ADD USER AUTHENTICATION",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBead_MixedCase(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add OAuth2 Support",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-oauth2-support", result)
}

func TestGenerateBranchNameFromBead_WithUnderscores(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "add_user_authentication",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBead_WithSpecialCharacters(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add user auth! (v2.0) [WIP]",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-auth-v20-wip", result)
}

func TestGenerateBranchNameFromBead_WithMultipleSpaces(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add   user    authentication",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBead_WithMultipleHyphens(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add---user---auth",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-auth", result)
}

func TestGenerateBranchNameFromBead_LeadingTrailingSpecialChars(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "  --Add user auth--  ",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-auth", result)
}

func TestGenerateBranchNameFromBead_WithNumbers(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add support for HTTP2",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-support-for-http2", result)
}

func TestGenerateBranchNameFromBead_OnlyNumbers(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "123456",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/123456", result)
}

func TestGenerateBranchNameFromBead_LongTitle_Truncates(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add comprehensive user authentication system with OAuth2 support and role-based access control",
	}

	result := GenerateBranchNameFromIssue(bead)

	// Should be truncated to 50 chars max (excluding feat/ prefix)
	assert.True(t, len(result) <= len("feat/")+50, "branch name should not exceed feat/ + 50 chars")
	assert.Equal(t, "feat/add-comprehensive-user-authentication-system-", result[:len("feat/add-comprehensive-user-authentication-system-")])
}

func TestGenerateBranchNameFromBead_LongTitle_NoTrailingHyphen(t *testing.T) {
	// Create a title that would end with a hyphen after truncation
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add comprehensive user authentication system with- more text here that will be cut off",
	}

	result := GenerateBranchNameFromIssue(bead)

	// Should not end with a hyphen (after the feat/ prefix)
	trimmedResult := result[len("feat/"):]
	assert.NotEqual(t, "-", string(trimmedResult[len(trimmedResult)-1]), "should not end with hyphen")
}

func TestGenerateBranchNameFromBead_EmptyTitle(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/", result)
}

func TestGenerateBranchNameFromBead_OnlySpecialChars(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "!@#$%^&*()",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/", result)
}

func TestGenerateBranchNameFromBead_OnlyWhitespace(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "     ",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/", result)
}

func TestGenerateBranchNameFromBead_Unicode(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add café support",
	}

	result := GenerateBranchNameFromIssue(bead)

	// Unicode characters (é) should be removed
	assert.Equal(t, "feat/add-caf-support", result)
}

func TestGenerateBranchNameFromBead_MixedUnderscoresAndSpaces(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "add_user authentication_system",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-user-authentication-system", result)
}

func TestGenerateBranchNameFromBead_ExactlyFiftyChars(t *testing.T) {
	// Create a title that results in exactly 50 chars
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add user authentication system for the application",
	}

	result := GenerateBranchNameFromIssue(bead)
	titlePart := result[len("feat/"):]

	assert.True(t, len(titlePart) <= 50, "title part should be at most 50 chars")
}

func TestGenerateBranchNameFromBead_PrefixIsCorrect(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Any title",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.True(t, len(result) >= 5, "result should have feat/ prefix")
	assert.Equal(t, "feat/", result[:5], "should have feat/ prefix")
}

func TestGenerateBranchNameFromBead_SingleWord(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Authentication",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/authentication", result)
}

func TestGenerateBranchNameFromBead_SingleCharacter(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "A",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/a", result)
}

func TestGenerateBranchNameFromBead_Colons(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "feat: add user authentication",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/feat-add-user-authentication", result)
}

func TestGenerateBranchNameFromBead_SlashesInTitle(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add user/admin authentication",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-useradmin-authentication", result)
}

func TestGenerateBranchNameFromBead_Apostrophes(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Fix user's profile page",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/fix-users-profile-page", result)
}

func TestGenerateBranchNameFromBead_Quotes(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: `Add "hello world" feature`,
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-hello-world-feature", result)
}

func TestGenerateBranchNameFromBead_Ampersand(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add search & filter",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-search-filter", result)
}

func TestGenerateBranchNameFromBead_PlusSign(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add C++ support",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-c-support", result)
}

func TestGenerateBranchNameFromBead_AtSign(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add @mentions support",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-mentions-support", result)
}

func TestGenerateBranchNameFromBead_HashSign(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add #hashtag support",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-hashtag-support", result)
}

func TestGenerateBranchNameFromBead_Dollars(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add $currency display",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-currency-display", result)
}

func TestGenerateBranchNameFromBead_Percent(t *testing.T) {
	bead := &beans.Bean{
		ID:    "test-1",
		Title: "Add 50% discount feature",
	}

	result := GenerateBranchNameFromIssue(bead)

	assert.Equal(t, "feat/add-50-discount-feature", result)
}

func TestCollectIssueIDsForAutomatedWorkflow_NoBeadsAvailable(t *testing.T) {
	// This test verifies error handling when the beads client is nil
	_, err := CollectIssueIDsForAutomatedWorkflow(context.Background(), "non-existent-bead-id", nil)

	// Should return an error since the client is nil
	assert.Error(t, err)
}

func TestParseBeanIDs_Single(t *testing.T) {
	result := ParseBeanIDs("bead-1")
	assert.Equal(t, []string{"bead-1"}, result)
}

func TestParseBeanIDs_Multiple(t *testing.T) {
	result := ParseBeanIDs("bead-1,bead-2,bead-3")
	assert.Equal(t, []string{"bead-1", "bead-2", "bead-3"}, result)
}

func TestParseBeanIDs_WithWhitespace(t *testing.T) {
	result := ParseBeanIDs("bead-1, bead-2 , bead-3")
	assert.Equal(t, []string{"bead-1", "bead-2", "bead-3"}, result)
}

func TestParseBeanIDs_Empty(t *testing.T) {
	result := ParseBeanIDs("")
	assert.Nil(t, result)
}

func TestParseBeanIDs_OnlyCommas(t *testing.T) {
	result := ParseBeanIDs(",,,")
	assert.Empty(t, result)
}

func TestParseBeanIDs_EmptyEntries(t *testing.T) {
	result := ParseBeanIDs("bead-1,,bead-2,")
	assert.Equal(t, []string{"bead-1", "bead-2"}, result)
}

func TestGenerateBranchNameFromBeads_Single(t *testing.T) {
	beadList := []*beans.Bean{
		{ID: "test-1", Title: "Add user authentication"},
	}

	result := GenerateBranchNameFromIssues(beadList)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBeads_Multiple(t *testing.T) {
	beadList := []*beans.Bean{
		{ID: "test-1", Title: "Fix bug"},
		{ID: "test-2", Title: "Add test"},
	}

	result := GenerateBranchNameFromIssues(beadList)

	assert.Equal(t, "feat/fix-bug-and-add-test", result)
}

func TestGenerateBranchNameFromBeads_MultipleTruncated(t *testing.T) {
	beadList := []*beans.Bean{
		{ID: "test-1", Title: "Add comprehensive user authentication"},
		{ID: "test-2", Title: "Add role based access control"},
	}

	result := GenerateBranchNameFromIssues(beadList)

	// Should be truncated to 50 chars max (excluding feat/ prefix)
	titlePart := result[len("feat/"):]
	assert.True(t, len(titlePart) <= 50, "title part should be at most 50 chars")
	assert.NotEqual(t, "-", string(titlePart[len(titlePart)-1]), "should not end with hyphen")
}

func TestGenerateBranchNameFromBeads_Empty(t *testing.T) {
	result := GenerateBranchNameFromIssues([]*beans.Bean{})

	assert.Equal(t, "feat/automated-work", result)
}

func TestGenerateBranchNameFromBeads_Nil(t *testing.T) {
	result := GenerateBranchNameFromIssues(nil)

	assert.Equal(t, "feat/automated-work", result)
}
