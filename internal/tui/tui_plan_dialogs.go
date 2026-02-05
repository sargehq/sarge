package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Dialog update handlers

func (m *planModel) updateBeadSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Esc or Ctrl+G cancels search and clears filter
	if msg.Type == tea.KeyEsc || msg.String() == "esc" || msg.String() == "escape" || msg.String() == "ctrl+g" {
		m.viewMode = ViewNormal
		m.textInput.Blur()
		m.filters.searchText = ""
		m.searchSeq++ // Increment to invalidate any in-flight searches
		return m, m.refreshData()
	}
	switch msg.String() {
	case "enter":
		// Confirm search and exit search mode, keeping the filter
		m.viewMode = ViewNormal
		m.textInput.Blur()
		m.filters.searchText = m.textInput.Value()
		return m, nil // No need to refresh, already filtered incrementally
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		// Apply incremental filtering as user types
		prevSearch := m.filters.searchText
		m.filters.searchText = m.textInput.Value()
		if m.filters.searchText != prevSearch {
			m.beadsCursor = 0 // Reset cursor when search changes
			m.searchSeq++     // Increment to invalidate any in-flight searches
			// Trigger data refresh to apply filter
			return m, tea.Batch(cmd, m.refreshData())
		}
		return m, cmd
	}
}

func (m *planModel) updateLabelFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || msg.String() == "esc" || msg.String() == "escape" {
		m.viewMode = ViewNormal
		m.textInput.Blur()
		return m, nil
	}
	switch msg.String() {
	case "enter":
		m.viewMode = ViewNormal
		m.filters.label = m.textInput.Value()
		return m, m.refreshData()
	default:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}
}

func (m *planModel) updateCloseBeadConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || msg.String() == "esc" || msg.String() == "escape" {
		m.viewMode = ViewNormal
		return m, nil
	}
	switch msg.String() {
	case "y", "Y":
		// Collect selected beads
		var beadIDs []string
		for _, item := range m.beadItems {
			if m.selectedBeads[item.ID] {
				beadIDs = append(beadIDs, item.ID)
			}
		}

		// If no selected beads, use cursor bead
		if len(beadIDs) == 0 && len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			beadIDs = append(beadIDs, m.beadItems[m.beadsCursor].ID)
		}

		m.viewMode = ViewNormal
		if len(beadIDs) == 1 {
			// Single bead - use the existing closeBead function
			return m, m.closeBead(beadIDs[0])
		} else if len(beadIDs) > 1 {
			// Multiple beads - use the batch close function
			return m, m.closeBeads(beadIDs)
		}
		return m, nil
	case "n", "N":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

func (m *planModel) updateDeleteBeadConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || msg.String() == "esc" || msg.String() == "escape" {
		m.viewMode = ViewNormal
		return m, nil
	}
	switch msg.String() {
	case "y", "Y":
		// Collect selected beads
		var beadIDs []string
		for _, item := range m.beadItems {
			if m.selectedBeads[item.ID] {
				beadIDs = append(beadIDs, item.ID)
			}
		}

		// If no selected beads, use cursor bead
		if len(beadIDs) == 0 && len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
			beadIDs = append(beadIDs, m.beadItems[m.beadsCursor].ID)
		}

		m.viewMode = ViewNormal
		// Clear selection after deletion
		m.selectedBeads = make(map[string]bool)
		if len(beadIDs) == 1 {
			// Single bead - use the deleteBead function
			return m, m.deleteBead(beadIDs[0])
		} else if len(beadIDs) > 1 {
			// Multiple beads - use the batch delete function
			return m, m.deleteBeads(beadIDs)
		}
		return m, nil
	case "n", "N":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

// Dialog render helpers

func (m *planModel) renderLabelFilterDialogContent() string {
	currentLabel := m.filters.label
	if currentLabel == "" {
		currentLabel = "(none)"
	}

	content := fmt.Sprintf(`
  Filter by Label

  Current: %s

  Enter label name (empty to clear):
  %s

  [Enter] Apply  [Esc] Cancel
`, currentLabel, m.textInput.View())

	return tuiDialogStyle.Render(content)
}

func (m *planModel) renderCloseBeadConfirmContent() string {
	// Collect selected beads
	var selectedBeads []beadItem
	for _, item := range m.beadItems {
		if m.selectedBeads[item.ID] {
			selectedBeads = append(selectedBeads, item)
		}
	}

	// If no selected beads, use cursor bead
	if len(selectedBeads) == 0 && len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
		selectedBeads = append(selectedBeads, m.beadItems[m.beadsCursor])
	}

	// Build the confirmation message
	var beadsList string
	if len(selectedBeads) == 1 {
		beadsList = fmt.Sprintf("  %s\n  %s", selectedBeads[0].ID, selectedBeads[0].Title)
	} else if len(selectedBeads) > 1 {
		beadsList = fmt.Sprintf("  %d issues:\n", len(selectedBeads))
		for i, bead := range selectedBeads {
			if i < 5 { // Show first 5 beads
				beadsList += fmt.Sprintf("  - %s: %s\n", bead.ID, bead.Title)
			}
		}
		if len(selectedBeads) > 5 {
			beadsList += fmt.Sprintf("  ... and %d more", len(selectedBeads)-5)
		}
	}

	var title string
	if len(selectedBeads) == 1 {
		title = "Close Issue"
	} else {
		title = fmt.Sprintf("Close %d Issues", len(selectedBeads))
	}

	content := fmt.Sprintf(`
  %s

  Are you sure you want to close:
%s

  [y] Yes  [n] No
`, title, beadsList)

	return tuiDialogStyle.Render(content)
}

func (m *planModel) renderDeleteBeadConfirmContent() string {
	// Collect selected beads
	var selectedBeads []beadItem
	for _, item := range m.beadItems {
		if m.selectedBeads[item.ID] {
			selectedBeads = append(selectedBeads, item)
		}
	}

	// If no selected beads, use cursor bead
	if len(selectedBeads) == 0 && len(m.beadItems) > 0 && m.beadsCursor < len(m.beadItems) {
		selectedBeads = append(selectedBeads, m.beadItems[m.beadsCursor])
	}

	// Build the confirmation message
	var beadsList string
	if len(selectedBeads) == 1 {
		beadsList = fmt.Sprintf("  %s\n  %s", selectedBeads[0].ID, selectedBeads[0].Title)
	} else if len(selectedBeads) > 1 {
		beadsList = fmt.Sprintf("  %d issues:\n", len(selectedBeads))
		for i, bead := range selectedBeads {
			if i < 5 { // Show first 5 beads
				beadsList += fmt.Sprintf("  - %s: %s\n", bead.ID, bead.Title)
			}
		}
		if len(selectedBeads) > 5 {
			beadsList += fmt.Sprintf("  ... and %d more", len(selectedBeads)-5)
		}
	}

	var title string
	if len(selectedBeads) == 1 {
		title = "Delete Issue (PERMANENT)"
	} else {
		title = fmt.Sprintf("Delete %d Issues (PERMANENT)", len(selectedBeads))
	}

	content := fmt.Sprintf(`
  %s

  Are you sure you want to PERMANENTLY delete:
%s

  This action cannot be undone!

  [y] Yes  [n] No
`, title, beadsList)

	return tuiDialogStyle.Render(content)
}

func (m *planModel) renderDestroyConfirmContent() string {
	workID := m.focusedWorkID
	workName := workID

	// Try to get work name from focused work
	if focusedWork := m.workDetails.GetFocusedWork(); focusedWork != nil && focusedWork.Work.Name != "" {
		workName = focusedWork.Work.Name
	}

	content := fmt.Sprintf(`
  Destroy Work

  Are you sure you want to destroy:
  %s
  %s

  This will:
  - Remove the git worktree
  - Delete the work directory
  - Update database records

  [y] Yes  [n] No
`, workID, workName)

	return tuiDialogStyle.Render(content)
}

