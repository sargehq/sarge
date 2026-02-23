package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

// Dialog update handlers

func (m *planModel) updateBeanSearch(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			m.beansCursor = 0 // Reset cursor when search changes
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

func (m *planModel) updateCloseBeanConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || msg.String() == "esc" || msg.String() == "escape" {
		m.viewMode = ViewNormal
		return m, nil
	}
	switch msg.String() {
	case "y", "Y":
		// Collect selected beans
		var beanIDs []string
		for _, item := range m.beanItems {
			if m.selectedBeans[item.ID] {
				beanIDs = append(beanIDs, item.ID)
			}
		}

		// If no selected beans, use cursor bean
		if len(beanIDs) == 0 && len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			beanIDs = append(beanIDs, m.beanItems[m.beansCursor].ID)
		}

		m.viewMode = ViewNormal
		if len(beanIDs) == 1 {
			// Single bean - use the existing closeBean function
			return m, m.closeBean(beanIDs[0])
		} else if len(beanIDs) > 1 {
			// Multiple beans - use the batch close function
			return m, m.closeBeans(beanIDs)
		}
		return m, nil
	case "n", "N":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

func (m *planModel) updateDeleteBeanConfirm(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if msg.Type == tea.KeyEsc || msg.String() == "esc" || msg.String() == "escape" {
		m.viewMode = ViewNormal
		return m, nil
	}
	switch msg.String() {
	case "y", "Y":
		// Collect selected beans
		var beanIDs []string
		for _, item := range m.beanItems {
			if m.selectedBeans[item.ID] {
				beanIDs = append(beanIDs, item.ID)
			}
		}

		// If no selected beans, use cursor bean
		if len(beanIDs) == 0 && len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
			beanIDs = append(beanIDs, m.beanItems[m.beansCursor].ID)
		}

		m.viewMode = ViewNormal
		// Clear selection after deletion
		m.selectedBeans = make(map[string]bool)
		if len(beanIDs) == 1 {
			// Single bean - use the deleteBean function
			return m, m.deleteBean(beanIDs[0])
		} else if len(beanIDs) > 1 {
			// Multiple beans - use the batch delete function
			return m, m.deleteBeans(beanIDs)
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

func (m *planModel) renderCloseBeanConfirmContent() string {
	// Collect selected beans
	var selectedBeans []beanItem
	for _, item := range m.beanItems {
		if m.selectedBeans[item.ID] {
			selectedBeans = append(selectedBeans, item)
		}
	}

	// If no selected beans, use cursor bean
	if len(selectedBeans) == 0 && len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
		selectedBeans = append(selectedBeans, m.beanItems[m.beansCursor])
	}

	// Build the confirmation message
	var beansList string
	if len(selectedBeans) == 1 {
		beansList = fmt.Sprintf("  %s\n  %s", selectedBeans[0].ID, selectedBeans[0].Title)
	} else if len(selectedBeans) > 1 {
		beansList = fmt.Sprintf("  %d issues:\n", len(selectedBeans))
		for i, bean := range selectedBeans {
			if i < 5 { // Show first 5 beans
				beansList += fmt.Sprintf("  - %s: %s\n", bean.ID, bean.Title)
			}
		}
		if len(selectedBeans) > 5 {
			beansList += fmt.Sprintf("  ... and %d more", len(selectedBeans)-5)
		}
	}

	var title string
	if len(selectedBeans) == 1 {
		title = "Close Issue"
	} else {
		title = fmt.Sprintf("Close %d Issues", len(selectedBeans))
	}

	content := fmt.Sprintf(`
  %s

  Are you sure you want to close:
%s

  [y] Yes  [n] No
`, title, beansList)

	return tuiDialogStyle.Render(content)
}

func (m *planModel) renderDeleteBeanConfirmContent() string {
	// Collect selected beans
	var selectedBeans []beanItem
	for _, item := range m.beanItems {
		if m.selectedBeans[item.ID] {
			selectedBeans = append(selectedBeans, item)
		}
	}

	// If no selected beans, use cursor bean
	if len(selectedBeans) == 0 && len(m.beanItems) > 0 && m.beansCursor < len(m.beanItems) {
		selectedBeans = append(selectedBeans, m.beanItems[m.beansCursor])
	}

	// Build the confirmation message
	var beansList string
	if len(selectedBeans) == 1 {
		beansList = fmt.Sprintf("  %s\n  %s", selectedBeans[0].ID, selectedBeans[0].Title)
	} else if len(selectedBeans) > 1 {
		beansList = fmt.Sprintf("  %d issues:\n", len(selectedBeans))
		for i, bean := range selectedBeans {
			if i < 5 { // Show first 5 beans
				beansList += fmt.Sprintf("  - %s: %s\n", bean.ID, bean.Title)
			}
		}
		if len(selectedBeans) > 5 {
			beansList += fmt.Sprintf("  ... and %d more", len(selectedBeans)-5)
		}
	}

	var title string
	if len(selectedBeans) == 1 {
		title = "Delete Issue (PERMANENT)"
	} else {
		title = fmt.Sprintf("Delete %d Issues (PERMANENT)", len(selectedBeans))
	}

	content := fmt.Sprintf(`
  %s

  Are you sure you want to PERMANENTLY delete:
%s

  This action cannot be undone!

  [y] Yes  [n] No
`, title, beansList)

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

