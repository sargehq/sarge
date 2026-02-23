package work

import (
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/stretchr/testify/assert"
)

func TestGenerateBranchNameFromBean_BasicTitle(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add user authentication",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBean_UppercaseTitle(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "ADD USER AUTHENTICATION",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBean_MixedCase(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add OAuth2 Support",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-oauth2-support", result)
}

func TestGenerateBranchNameFromBean_WithUnderscores(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "add_user_authentication",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBean_WithSpecialCharacters(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add user auth! (v2.0) [WIP]",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-auth-v20-wip", result)
}

func TestGenerateBranchNameFromBean_WithMultipleSpaces(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add   user    authentication",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBean_WithMultipleHyphens(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add---user---auth",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-auth", result)
}

func TestGenerateBranchNameFromBean_LeadingTrailingSpecialChars(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "  --Add user auth--  ",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-auth", result)
}

func TestGenerateBranchNameFromBean_WithNumbers(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add support for HTTP2",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-support-for-http2", result)
}

func TestGenerateBranchNameFromBean_OnlyNumbers(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "123456",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/123456", result)
}

func TestGenerateBranchNameFromBean_LongTitle_Truncates(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add comprehensive user authentication system with OAuth2 support and role-based access control",
	}

	result := GenerateBranchNameFromIssue(bean)

	// Should be truncated to 50 chars max (excluding feat/ prefix)
	assert.True(t, len(result) <= len("feat/")+50, "branch name should not exceed feat/ + 50 chars")
	assert.Equal(t, "feat/add-comprehensive-user-authentication-system-", result[:len("feat/add-comprehensive-user-authentication-system-")])
}

func TestGenerateBranchNameFromBean_LongTitle_NoTrailingHyphen(t *testing.T) {
	// Create a title that would end with a hyphen after truncation
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add comprehensive user authentication system with- more text here that will be cut off",
	}

	result := GenerateBranchNameFromIssue(bean)

	// Should not end with a hyphen (after the feat/ prefix)
	trimmedResult := result[len("feat/"):]
	assert.NotEqual(t, "-", string(trimmedResult[len(trimmedResult)-1]), "should not end with hyphen")
}

func TestGenerateBranchNameFromBean_EmptyTitle(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/", result)
}

func TestGenerateBranchNameFromBean_OnlySpecialChars(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "!@#$%^&*()",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/", result)
}

func TestGenerateBranchNameFromBean_OnlyWhitespace(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "     ",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/", result)
}

func TestGenerateBranchNameFromBean_Unicode(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add café support",
	}

	result := GenerateBranchNameFromIssue(bean)

	// Unicode characters (é) should be removed
	assert.Equal(t, "feat/add-caf-support", result)
}

func TestGenerateBranchNameFromBean_MixedUnderscoresAndSpaces(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "add_user authentication_system",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-user-authentication-system", result)
}

func TestGenerateBranchNameFromBean_ExactlyFiftyChars(t *testing.T) {
	// Create a title that results in exactly 50 chars
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add user authentication system for the application",
	}

	result := GenerateBranchNameFromIssue(bean)
	titlePart := result[len("feat/"):]

	assert.True(t, len(titlePart) <= 50, "title part should be at most 50 chars")
}

func TestGenerateBranchNameFromBean_PrefixIsCorrect(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Any title",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.True(t, len(result) >= 5, "result should have feat/ prefix")
	assert.Equal(t, "feat/", result[:5], "should have feat/ prefix")
}

func TestGenerateBranchNameFromBean_SingleWord(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Authentication",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/authentication", result)
}

func TestGenerateBranchNameFromBean_SingleCharacter(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "A",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/a", result)
}

func TestGenerateBranchNameFromBean_Colons(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "feat: add user authentication",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/feat-add-user-authentication", result)
}

func TestGenerateBranchNameFromBean_SlashesInTitle(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add user/admin authentication",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-useradmin-authentication", result)
}

func TestGenerateBranchNameFromBean_Apostrophes(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Fix user's profile page",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/fix-users-profile-page", result)
}

func TestGenerateBranchNameFromBean_Quotes(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: `Add "hello world" feature`,
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-hello-world-feature", result)
}

func TestGenerateBranchNameFromBean_Ampersand(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add search & filter",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-search-filter", result)
}

func TestGenerateBranchNameFromBean_PlusSign(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add C++ support",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-c-support", result)
}

func TestGenerateBranchNameFromBean_AtSign(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add @mentions support",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-mentions-support", result)
}

func TestGenerateBranchNameFromBean_HashSign(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add #hashtag support",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-hashtag-support", result)
}

func TestGenerateBranchNameFromBean_Dollars(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add $currency display",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-currency-display", result)
}

func TestGenerateBranchNameFromBean_Percent(t *testing.T) {
	bean := &beans.Bean{
		ID:    "test-1",
		Title: "Add 50% discount feature",
	}

	result := GenerateBranchNameFromIssue(bean)

	assert.Equal(t, "feat/add-50-discount-feature", result)
}

func TestCollectIssueIDsForAutomatedWorkflow_NoBeansAvailable(t *testing.T) {
	// This test verifies error handling when the beans client is nil
	_, err := CollectIssueIDsForAutomatedWorkflow(context.Background(), "non-existent-bean-id", nil)

	// Should return an error since the client is nil
	assert.Error(t, err)
}

func TestParseBeanIDs_Single(t *testing.T) {
	result := ParseBeanIDs("bean-1")
	assert.Equal(t, []string{"bean-1"}, result)
}

func TestParseBeanIDs_Multiple(t *testing.T) {
	result := ParseBeanIDs("bean-1,bean-2,bean-3")
	assert.Equal(t, []string{"bean-1", "bean-2", "bean-3"}, result)
}

func TestParseBeanIDs_WithWhitespace(t *testing.T) {
	result := ParseBeanIDs("bean-1, bean-2 , bean-3")
	assert.Equal(t, []string{"bean-1", "bean-2", "bean-3"}, result)
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
	result := ParseBeanIDs("bean-1,,bean-2,")
	assert.Equal(t, []string{"bean-1", "bean-2"}, result)
}

func TestGenerateBranchNameFromBeans_Single(t *testing.T) {
	beanList := []*beans.Bean{
		{ID: "test-1", Title: "Add user authentication"},
	}

	result := GenerateBranchNameFromIssues(beanList)

	assert.Equal(t, "feat/add-user-authentication", result)
}

func TestGenerateBranchNameFromBeans_Multiple(t *testing.T) {
	beanList := []*beans.Bean{
		{ID: "test-1", Title: "Fix bug"},
		{ID: "test-2", Title: "Add test"},
	}

	result := GenerateBranchNameFromIssues(beanList)

	assert.Equal(t, "feat/fix-bug-and-add-test", result)
}

func TestGenerateBranchNameFromBeans_MultipleTruncated(t *testing.T) {
	beanList := []*beans.Bean{
		{ID: "test-1", Title: "Add comprehensive user authentication"},
		{ID: "test-2", Title: "Add role based access control"},
	}

	result := GenerateBranchNameFromIssues(beanList)

	// Should be truncated to 50 chars max (excluding feat/ prefix)
	titlePart := result[len("feat/"):]
	assert.True(t, len(titlePart) <= 50, "title part should be at most 50 chars")
	assert.NotEqual(t, "-", string(titlePart[len(titlePart)-1]), "should not end with hyphen")
}

func TestGenerateBranchNameFromBeans_Empty(t *testing.T) {
	result := GenerateBranchNameFromIssues([]*beans.Bean{})

	assert.Equal(t, "feat/automated-work", result)
}

func TestGenerateBranchNameFromBeans_Nil(t *testing.T) {
	result := GenerateBranchNameFromIssues(nil)

	assert.Equal(t, "feat/automated-work", result)
}
