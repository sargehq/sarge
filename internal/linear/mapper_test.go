package linear

import (
	"strings"
	"testing"

	"github.com/sargehq/sarge/internal/beans"
	"github.com/stretchr/testify/require"
)

func TestMapStatus(t *testing.T) {
	tests := []struct {
		name  string
		state State
		want  string
	}{
		{
			name:  "unstarted to open",
			state: State{Type: "unstarted"},
			want:  beans.StatusTodo,
		},
		{
			name:  "started to in_progress",
			state: State{Type: "started"},
			want:  beans.StatusInProgress,
		},
		{
			name:  "completed to closed",
			state: State{Type: "completed"},
			want:  beans.StatusCompleted,
		},
		{
			name:  "canceled to scrapped",
			state: State{Type: "canceled"},
			want:  beans.StatusScrapped,
		},
		{
			name:  "unknown to open",
			state: State{Type: "unknown"},
			want:  beans.StatusTodo,
		},
		{
			name:  "case insensitive",
			state: State{Type: "STARTED"},
			want:  beans.StatusInProgress,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapStatus(tt.state)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMapPriority(t *testing.T) {
	tests := []struct {
		name     string
		priority int
		want     string
	}{
		{
			name:     "0 to P0 (urgent to critical)",
			priority: 0,
			want:     "P0",
		},
		{
			name:     "1 to P1 (high to high)",
			priority: 1,
			want:     "P1",
		},
		{
			name:     "2 to P2 (medium to medium)",
			priority: 2,
			want:     "P2",
		},
		{
			name:     "3 to P3 (low to low)",
			priority: 3,
			want:     "P3",
		},
		{
			name:     "4 to P4 (no priority to backlog)",
			priority: 4,
			want:     "P4",
		},
		{
			name:     "unknown to P2 (default)",
			priority: 99,
			want:     "P2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapPriority(tt.priority)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMapType(t *testing.T) {
	tests := []struct {
		name  string
		issue *Issue
		want  string
	}{
		{
			name: "bug label",
			issue: &Issue{
				Title:  "Some issue",
				Labels: []Label{{Name: "bug"}},
			},
			want: "bug",
		},
		{
			name: "feature label",
			issue: &Issue{
				Title:  "Some issue",
				Labels: []Label{{Name: "feature"}},
			},
			want: "feature",
		},
		{
			name: "bug in title",
			issue: &Issue{
				Title: "Fix bug in authentication",
			},
			want: "bug",
		},
		{
			name: "feature in title",
			issue: &Issue{
				Title: "Add feature for user profile",
			},
			want: "feature",
		},
		{
			name: "default to task",
			issue: &Issue{
				Title: "Update documentation",
			},
			want: "task",
		},
		{
			name: "fix in label",
			issue: &Issue{
				Title:  "Something broken",
				Labels: []Label{{Name: "hotfix"}},
			},
			want: "bug",
		},
		{
			name: "enhancement in label",
			issue: &Issue{
				Title:  "Improve performance",
				Labels: []Label{{Name: "enhancement"}},
			},
			want: "feature",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapType(tt.issue)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestMapIssueToBeadCreate(t *testing.T) {
	estimate := 3.5
	issue := &Issue{
		Identifier:  "ENG-123",
		Title:       "Fix authentication bug",
		Description: "Users cannot log in",
		Priority:    1,
		State:       State{Type: "started"},
		URL:         "https://linear.app/team/issue/ENG-123",
		Assignee:    &User{Name: "John Doe", Email: "john@example.com"},
		Project:     &Project{Name: "Q1 Features"},
		Labels:      []Label{{Name: "bug"}, {Name: "security"}},
		Estimate:    &estimate,
	}

	opts := MapIssueToBeadCreate(issue)

	require.Equal(t, "Fix authentication bug", opts.Title)
	require.Equal(t, "bug", opts.Type)
	require.Equal(t, "P1", opts.Priority)
	require.Equal(t, beans.StatusInProgress, opts.Status)
	require.Equal(t, "john@example.com", opts.Assignee)
	require.Len(t, opts.Labels, 2)
	require.Equal(t, "ENG-123", opts.Metadata["linear_id"])
	require.Equal(t, "https://linear.app/team/issue/ENG-123", opts.Metadata["linear_url"])
	require.Equal(t, "Q1 Features", opts.Metadata["linear_project"])
	require.Equal(t, "3.5", opts.Metadata["linear_estimate"])
}

func TestFormatBeadDescription(t *testing.T) {
	estimate := 2.0
	issue := &Issue{
		Identifier:  "ENG-456",
		Title:       "Add feature",
		Description: "Original description",
		State:       State{Name: "In Progress", Type: "started"},
		URL:         "https://linear.app/team/issue/ENG-456",
		Project:     &Project{Name: "Backend"},
		Estimate:    &estimate,
		Assignee:    &User{Name: "Jane Smith"},
	}

	desc := FormatBeadDescription(issue)

	// Check that it contains key elements
	require.True(t, strings.Contains(desc, "Original description"), "Description should contain original description")
	require.True(t, strings.Contains(desc, "ENG-456"), "Description should contain Linear ID")
	require.True(t, strings.Contains(desc, "https://linear.app/team/issue/ENG-456"), "Description should contain URL")
	require.True(t, strings.Contains(desc, "In Progress"), "Description should contain state name")
	require.True(t, strings.Contains(desc, "Backend"), "Description should contain project name")
	require.True(t, strings.Contains(desc, "2.0"), "Description should contain estimate")
	require.True(t, strings.Contains(desc, "Jane Smith"), "Description should contain assignee name")
}
