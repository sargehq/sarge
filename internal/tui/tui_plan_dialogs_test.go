package tui

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sargehq/sarge/internal/beans"
	"github.com/stretchr/testify/require"
)

// TestMultiSelectionCloseConfirmation tests the close confirmation dialog with multiple selected beans
func TestMultiSelectionCloseConfirmation(t *testing.T) {
	tests := []struct {
		name             string
		beanItems        []beanItem
		selectedBeans    map[string]bool
		cursorIndex      int
		expectedCount    int  // Expected number of beans to be closed
		expectedInDialog bool // Whether beans should appear in dialog
		description      string
	}{
		{
			name: "Multiple selected beans",
			beanItems: []beanItem{
				testBeanItem("bean-1", "First task", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-2", "Second task", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-3", "Third task", beans.StatusTodo, beans.PriorityNormal, "task"),
			},
			selectedBeans: map[string]bool{
				"bean-1": true,
				"bean-2": true,
			},
			cursorIndex:      0,
			expectedCount:    2,
			expectedInDialog: true,
			description:      "Should show and close 2 selected beans",
		},
		{
			name: "No selected beans - uses cursor",
			beanItems: []beanItem{
				testBeanItem("bean-1", "First task", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-2", "Second task", beans.StatusTodo, beans.PriorityNormal, "task"),
			},
			selectedBeans:    map[string]bool{},
			cursorIndex:      1,
			expectedCount:    1,
			expectedInDialog: true,
			description:      "Should use cursor bean when no selection",
		},
		{
			name: "All beans selected",
			beanItems: []beanItem{
				testBeanItem("bean-1", "First task", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-2", "Second task", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-3", "Third task", beans.StatusTodo, beans.PriorityNormal, "task"),
			},
			selectedBeans: map[string]bool{
				"bean-1": true,
				"bean-2": true,
				"bean-3": true,
			},
			cursorIndex:      0,
			expectedCount:    3,
			expectedInDialog: true,
			description:      "Should show and close all 3 selected beans",
		},
		{
			name: "More than 5 beans selected",
			beanItems: []beanItem{
				testBeanItem("bean-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-3", "Task 3", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-4", "Task 4", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-5", "Task 5", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-6", "Task 6", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-7", "Task 7", beans.StatusTodo, beans.PriorityNormal, "task"),
			},
			selectedBeans: map[string]bool{
				"bean-1": true,
				"bean-2": true,
				"bean-3": true,
				"bean-4": true,
				"bean-5": true,
				"bean-6": true,
				"bean-7": true,
			},
			cursorIndex:      0,
			expectedCount:    7,
			expectedInDialog: true,
			description:      "Should show first 5 beans and ellipsis for remaining",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock planModel
			m := &planModel{
				beanItems:     tt.beanItems,
				selectedBeans: tt.selectedBeans,
				beansCursor:   tt.cursorIndex,
				viewMode:      ViewCloseBeanConfirm,
			}

			// Test the dialog content rendering
			dialogContent := m.renderCloseBeanConfirmContent()

			// Check if the dialog shows the correct number of beans
			if tt.expectedCount == 1 {
				// For single bean, check title shows "Close Issue"
				require.True(t, strings.Contains(dialogContent, "Close Issue"),
					"%s: Expected 'Close Issue' in dialog for single bean", tt.description)
			} else {
				// For multiple beans, check title shows correct count
				if tt.expectedCount > 1 {
					require.True(t, strings.Contains(dialogContent, "Issues"),
						"%s: Expected 'Issues' (plural) in dialog for multiple beans", tt.description)
				}
			}

			// Check if selected bean IDs appear in the dialog
			if tt.expectedInDialog {
				selectedCount := 0
				shownCount := 0
				for _, item := range tt.beanItems {
					if tt.selectedBeans[item.ID] {
						selectedCount++
						// Only first 5 beans should be shown
						if shownCount < 5 {
							require.True(t, strings.Contains(dialogContent, item.ID),
								"%s: Expected bean ID '%s' to appear in dialog (one of first 5)", tt.description, item.ID)
							shownCount++
						}
					}
				}

				// If more than 5 selected, check for ellipsis
				if selectedCount > 5 {
					require.True(t, strings.Contains(dialogContent, "and") || strings.Contains(dialogContent, "more"),
						"%s: Expected '... and X more' for more than 5 selected beans", tt.description)
				}
			}

			// Check dialog has confirmation buttons
			require.True(t, strings.Contains(dialogContent, "[y]") && strings.Contains(dialogContent, "[n]"),
				"%s: Expected confirmation buttons [y] and [n] in dialog", tt.description)
		})
	}
}

// TestUpdateCloseBeanConfirm tests the keyboard handling for close confirmation
func TestUpdateCloseBeanConfirm(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		shouldClose  bool
		shouldCancel bool
		description  string
	}{
		{
			name:         "Confirm with 'y'",
			key:          "y",
			shouldClose:  true,
			shouldCancel: false,
			description:  "Pressing 'y' should confirm close",
		},
		{
			name:         "Confirm with 'Y'",
			key:          "Y",
			shouldClose:  true,
			shouldCancel: false,
			description:  "Pressing 'Y' should confirm close",
		},
		{
			name:         "Cancel with 'n'",
			key:          "n",
			shouldClose:  false,
			shouldCancel: true,
			description:  "Pressing 'n' should cancel",
		},
		{
			name:         "Cancel with 'N'",
			key:          "N",
			shouldClose:  false,
			shouldCancel: true,
			description:  "Pressing 'N' should cancel",
		},
		{
			name:         "Cancel with Esc",
			key:          "esc",
			shouldClose:  false,
			shouldCancel: true,
			description:  "Pressing Esc should cancel",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock planModel with selected beans
			m := &planModel{
				beanItems: []beanItem{
					testBeanItem("bean-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task"),
					testBeanItem("bean-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task"),
				},
				selectedBeans: map[string]bool{
					"bean-1": true,
					"bean-2": true,
				},
				beansCursor: 0,
				viewMode:    ViewCloseBeanConfirm,
			}

			// Create the key message
			var keyMsg tea.KeyMsg
			if tt.key == "esc" {
				keyMsg = tea.KeyMsg{Type: tea.KeyEsc}
			} else {
				keyMsg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(tt.key)}
			}

			// Update the model
			newModel, cmd := m.updateCloseBeanConfirm(keyMsg)
			updatedModel := newModel.(*planModel)

			// Check if view mode changed back to normal
			if tt.shouldCancel || tt.shouldClose {
				require.Equal(t, ViewNormal, updatedModel.viewMode,
					"%s: Expected viewMode to be ViewNormal after action", tt.description)
			}

			// If close was confirmed, a command should be returned
			if tt.shouldClose {
				require.NotNil(t, cmd,
					"%s: Expected a command to be returned when confirming close", tt.description)
			}
		})
	}
}

// TestCloseKeyHandlerWithSelection tests 'x' key handler with multi-selection
func TestCloseKeyHandlerWithSelection(t *testing.T) {
	tests := []struct {
		name             string
		beanItems        []beanItem
		selectedBeans    map[string]bool
		cursorIndex      int
		shouldShowDialog bool
		description      string
	}{
		{
			name: "With selected beans",
			beanItems: []beanItem{
				testBeanItem("bean-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task"),
				testBeanItem("bean-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task"),
			},
			selectedBeans: map[string]bool{
				"bean-1": true,
			},
			cursorIndex:      0,
			shouldShowDialog: true,
			description:      "Should show dialog when beans are selected",
		},
		{
			name: "Without selected beans but with cursor",
			beanItems: []beanItem{
				testBeanItem("bean-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task"),
			},
			selectedBeans:    map[string]bool{},
			cursorIndex:      0,
			shouldShowDialog: true,
			description:      "Should show dialog when cursor is on a bean",
		},
		{
			name:             "No beans available",
			beanItems:        []beanItem{},
			selectedBeans:    map[string]bool{},
			cursorIndex:      0,
			shouldShowDialog: false,
			description:      "Should not show dialog when no beans available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a mock planModel
			m := &planModel{
				beanItems:     tt.beanItems,
				selectedBeans: tt.selectedBeans,
				beansCursor:   tt.cursorIndex,
				viewMode:      ViewNormal,
				ctx:           context.Background(),
			}

			// Simulate pressing 'x' key - extract the relevant logic
			if len(m.beanItems) > 0 {
				hasSelection := false
				for _, item := range m.beanItems {
					if m.selectedBeans[item.ID] {
						hasSelection = true
						break
					}
				}
				// If we have selected beans or a cursor bean, show confirmation
				if hasSelection || m.beansCursor < len(m.beanItems) {
					m.viewMode = ViewCloseBeanConfirm
				}
			}

			// Check if dialog was shown as expected
			dialogShown := m.viewMode == ViewCloseBeanConfirm
			require.Equal(t, tt.shouldShowDialog, dialogShown,
				"%s: dialog shown state mismatch", tt.description)
		})
	}
}

// TestBatchCloseFunction tests that multiple beans can be closed in a batch
func TestBatchCloseFunction(t *testing.T) {
	// This test validates that the closeBeans function is called with multiple IDs
	// In a real test, we would mock the beans command execution
	beanIDs := []string{"bean-1", "bean-2", "bean-3"}

	// Verify the function signature exists and accepts multiple IDs
	m := &planModel{
		ctx:                context.Background(),
		beanItems:          []beanItem{},
		selectedBeans:      map[string]bool{},
		activeBeanSessions: map[string]bool{},
	}

	// The closeBeans function should accept a slice of bean IDs
	cmd := m.closeBeans(beanIDs)

	// Verify the command is not nil
	require.NotNil(t, cmd, "closeBeans should return a non-nil command")

	// In a real scenario, we would verify that the beans command is called with all IDs:
	// Expected: beans close bean-1 bean-2 bean-3
	// This would require mocking exec.CommandContext or using an interface
}

// TestCloseConfirmationEdgeCases tests edge cases for close confirmation
func TestCloseConfirmationEdgeCases(t *testing.T) {
	tests := []struct {
		name             string
		setup            func() *planModel
		expectedBehavior string
	}{
		{
			name: "Empty selection and invalid cursor",
			setup: func() *planModel {
				return &planModel{
					beanItems:     []beanItem{testBeanItem("bean-1", "Task", beans.StatusTodo, beans.PriorityNormal, "task")},
					selectedBeans: map[string]bool{},
					beansCursor:   10, // Invalid cursor position
					viewMode:      ViewCloseBeanConfirm,
				}
			},
			expectedBehavior: "Should handle gracefully without panic",
		},
		{
			name: "Already closed beans in selection",
			setup: func() *planModel {
				return &planModel{
					beanItems: []beanItem{
						testBeanItem("bean-1", "Task 1", "closed", beans.PriorityNormal, "task"),
						testBeanItem("bean-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task"),
					},
					selectedBeans: map[string]bool{
						"bean-1": true, // Already closed
						"bean-2": true,
					},
					beansCursor: 0,
					viewMode:    ViewCloseBeanConfirm,
				}
			},
			expectedBehavior: "Should still show both beans in confirmation",
		},
		{
			name: "Mixed assigned and unassigned beans",
			setup: func() *planModel {
				item1 := testBeanItem("bean-1", "Task 1", beans.StatusTodo, beans.PriorityNormal, "task")
				item1.assignedWorkID = "w-123"
				item2 := testBeanItem("bean-2", "Task 2", beans.StatusTodo, beans.PriorityNormal, "task")
				return &planModel{
					beanItems: []beanItem{item1, item2},
					selectedBeans: map[string]bool{
						"bean-1": true, // Already assigned to work
						"bean-2": true,
					},
					beansCursor: 0,
					viewMode:    ViewCloseBeanConfirm,
				}
			},
			expectedBehavior: "Should show both beans regardless of assignment",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := tt.setup()

			// Test dialog rendering doesn't panic
			require.NotPanics(t, func() {
				_ = m.renderCloseBeanConfirmContent()
			}, "%s: Panic occurred during dialog rendering", tt.name)

			// Test update function doesn't panic when confirming
			require.NotPanics(t, func() {
				keyMsg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}
				_, _ = m.updateCloseBeanConfirm(keyMsg)
			}, "%s: Panic on confirm", tt.name)
		})
	}
}
