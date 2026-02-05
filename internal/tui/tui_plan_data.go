package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sargehq/sarge/internal/beads"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/linear"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/work"
)

// refreshData creates a tea.Cmd that refreshes bead data
func (m *planModel) refreshData() tea.Cmd {
	// Capture current filter and sequence at creation time to avoid race conditions
	filters := m.filters
	seq := m.searchSeq
	return m.refreshDataWithFilters(filters, seq)
}

// refreshDataWithFilters creates a refresh command with captured filter values.
// This prevents race conditions when the user types quickly.
func (m *planModel) refreshDataWithFilters(filters beadFilters, seq uint64) tea.Cmd {
	return func() tea.Msg {
		items, err := m.loadBeadsWithFilters(filters)

		// Also fetch active sessions
		session := m.sessionName()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)

		return planDataMsg{
			beads:          items,
			activeSessions: activeSessions,
			err:            err,
			searchSeq:      seq,
		}
	}
}

func (m *planModel) loadBeads() ([]beadItem, error) {
	return m.loadBeadsWithFilters(m.filters)
}

// loadBeadsWithFilters loads beads using the provided filters.
// This allows capturing filters at command creation time to avoid race conditions.
func (m *planModel) loadBeadsWithFilters(filters beadFilters) ([]beadItem, error) {
	mainRepoPath := m.proj.MainRepoPath()

	// Handle task filter - show beads assigned to a specific task
	if filters.task != "" {
		return m.loadBeadsForTask(filters)
	}

	// Handle children filter - show children (dependents) of a specific bead
	if filters.children != "" {
		return m.loadBeadsForChildren(filters)
	}

	// Use the shared fetchBeadsWithFilters function
	items, err := fetchBeadsWithFilters(m.ctx, m.proj.Beads, mainRepoPath, filters)
	if err != nil {
		return nil, err
	}

	// Fetch assigned beads from database and populate assignedWorkID
	assignedBeads, err := m.proj.DB.GetAllAssignedBeads(m.ctx)
	if err == nil {
		for i := range items {
			if workID, ok := assignedBeads[items[i].ID]; ok {
				items[i].assignedWorkID = workID
			}
		}
	}

	// Build tree structure from dependencies
	items = buildBeadTree(m.ctx, items, m.proj.Beads)

	// If no tree structure, apply regular sorting
	hasTree := false
	for _, item := range items {
		if item.treeDepth > 0 || len(item.Dependents) > 0 {
			hasTree = true
			break
		}
	}

	if !hasTree {
		// Fall back to regular sorting if no tree structure
		switch filters.sortBy {
		case "priority":
			sort.Slice(items, func(i, j int) bool {
				return items[i].Priority < items[j].Priority
			})
		case "title":
			sort.Slice(items, func(i, j int) bool {
				return items[i].Title < items[j].Title
			})
		}
	}

	return items, nil
}

// loadBeadsForTask loads beads assigned to a specific task.
// This fetches all beads for the task regardless of status filter.
func (m *planModel) loadBeadsForTask(filters beadFilters) ([]beadItem, error) {
	// Get bead IDs assigned to this task from the database
	beadIDs, err := m.proj.DB.GetTaskBeads(m.ctx, filters.task)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beads: %w", err)
	}

	if len(beadIDs) == 0 {
		return nil, nil
	}

	// Fetch the beads from the beads client (uses cache)
	var items []beadItem
	for _, beadID := range beadIDs {
		bead, err := m.proj.Beads.GetBead(m.ctx, beadID)
		if err != nil || bead == nil {
			continue
		}
		items = append(items, beadItem{
			BeadWithDeps: bead,
		})
	}

	// Apply search text filter if set
	if filters.searchText != "" {
		searchLower := strings.ToLower(filters.searchText)
		var filtered []beadItem
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.ID), searchLower) ||
				strings.Contains(strings.ToLower(item.Title), searchLower) ||
				strings.Contains(strings.ToLower(item.Description), searchLower) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// Build tree structure from dependencies
	items = buildBeadTree(m.ctx, items, m.proj.Beads)

	return items, nil
}

// loadBeadsForChildren loads children (dependents) of a specific bead.
// This fetches all dependents regardless of status filter.
func (m *planModel) loadBeadsForChildren(filters beadFilters) ([]beadItem, error) {
	// Get the parent bead to find its dependents
	parentBead, err := m.proj.Beads.GetBead(m.ctx, filters.children)
	if err != nil {
		return nil, fmt.Errorf("failed to get bead: %w", err)
	}
	if parentBead == nil {
		return nil, nil
	}

	// Also include the parent bead itself
	items := []beadItem{{BeadWithDeps: parentBead}}

	// Fetch each dependent bead
	for _, dep := range parentBead.Dependents {
		bead, err := m.proj.Beads.GetBead(m.ctx, dep.IssueID)
		if err != nil || bead == nil {
			continue
		}
		items = append(items, beadItem{
			BeadWithDeps: bead,
		})
	}

	// Apply search text filter if set
	if filters.searchText != "" {
		searchLower := strings.ToLower(filters.searchText)
		var filtered []beadItem
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.ID), searchLower) ||
				strings.Contains(strings.ToLower(item.Title), searchLower) ||
				strings.Contains(strings.ToLower(item.Description), searchLower) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// Build tree structure from dependencies
	items = buildBeadTree(m.ctx, items, m.proj.Beads)

	return items, nil
}

func (m *planModel) createBead(title, beadType string, priority int, isEpic bool, description string, parent string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		beadsPath := m.proj.BeadsPath()

		beadID, err := beads.NewCLI(beadsPath).Create(ctx, beads.CreateOptions{
			Title:       title,
			Type:        beadType,
			Priority:    priority,
			IsEpic:      isEpic,
			Description: description,
			Parent:      parent,
		})
		if err != nil {
			return planDataMsg{err: fmt.Errorf("failed to create issue: %w", err)}
		}

		// Refresh after creation
		items, err := m.loadBeads()
		session := m.sessionName()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)

		return planDataMsg{beads: items, activeSessions: activeSessions, err: err, createdBeadID: beadID}
	}
}

func (m *planModel) closeBead(beadID string) tea.Cmd {
	return func() tea.Msg {
		beadsPath := m.proj.BeadsPath()
		session := m.sessionName()
		tabName := db.TabNameForBead(beadID)

		// If there's an active session for this bead, close it
		if m.activeBeadSessions[beadID] {
			// Terminate and close the tab
			_ = m.zj.Session(session).TerminateAndCloseTab(m.ctx, tabName)
			// Unregister from database
			_ = m.proj.DB.UnregisterPlanSession(m.ctx, beadID)
		}

		// Close the bead
		if err := beads.NewCLI(beadsPath).Close(m.ctx, beadID); err != nil {
			return planDataMsg{err: fmt.Errorf("failed to close issue: %w", err)}
		}

		// Refresh after close
		items, err := m.loadBeads()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)
		return planDataMsg{beads: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) closeBeads(beadIDs []string) tea.Cmd {
	return func() tea.Msg {
		beadsPath := m.proj.BeadsPath()
		session := m.sessionName()

		// First, close any active sessions for these beads
		zjSession := m.zj.Session(session)
		for _, beadID := range beadIDs {
			if m.activeBeadSessions[beadID] {
				tabName := db.TabNameForBead(beadID)
				// Terminate and close the tab
				_ = zjSession.TerminateAndCloseTab(m.ctx, tabName)
				// Unregister from database
				_ = m.proj.DB.UnregisterPlanSession(m.ctx, beadID)
			}
		}

		// Close all beads using the beads package
		cli := beads.NewCLI(beadsPath)
		for _, beadID := range beadIDs {
			if err := cli.Close(m.ctx, beadID); err != nil {
				return planDataMsg{err: fmt.Errorf("failed to close issue %s: %w", beadID, err)}
			}
		}

		// Refresh after close
		items, err := m.loadBeads()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)
		return planDataMsg{beads: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) deleteBead(beadID string) tea.Cmd {
	return func() tea.Msg {
		beadsPath := m.proj.BeadsPath()
		session := m.sessionName()
		tabName := db.TabNameForBead(beadID)

		// If there's an active session for this bead, close it
		if m.activeBeadSessions[beadID] {
			// Terminate and close the tab
			_ = m.zj.Session(session).TerminateAndCloseTab(m.ctx, tabName)
			// Unregister from database
			_ = m.proj.DB.UnregisterPlanSession(m.ctx, beadID)
		}

		// Delete the bead permanently with --force to skip confirmation
		if err := beads.NewCLI(beadsPath).Delete(m.ctx, beadID, true); err != nil {
			return planDataMsg{err: fmt.Errorf("failed to delete issue: %w", err)}
		}

		// Refresh after delete
		items, err := m.loadBeads()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)
		return planDataMsg{beads: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) deleteBeads(beadIDs []string) tea.Cmd {
	return func() tea.Msg {
		beadsPath := m.proj.BeadsPath()
		session := m.sessionName()

		// First, close any active sessions for these beads
		zjSession := m.zj.Session(session)
		for _, beadID := range beadIDs {
			if m.activeBeadSessions[beadID] {
				tabName := db.TabNameForBead(beadID)
				// Terminate and close the tab
				_ = zjSession.TerminateAndCloseTab(m.ctx, tabName)
				// Unregister from database
				_ = m.proj.DB.UnregisterPlanSession(m.ctx, beadID)
			}
		}

		// Delete all beads permanently with --force
		cli := beads.NewCLI(beadsPath)
		for _, beadID := range beadIDs {
			if err := cli.Delete(m.ctx, beadID, true); err != nil {
				return planDataMsg{err: fmt.Errorf("failed to delete issue %s: %w", beadID, err)}
			}
		}

		// Refresh after delete
		items, err := m.loadBeads()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)
		return planDataMsg{beads: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) saveBeadEdit(beadID, title, description, beadType, status string) tea.Cmd {
	return func() tea.Msg {
		beadsPath := m.proj.BeadsPath()

		// Update the bead using beads package
		err := beads.NewCLI(beadsPath).Update(m.ctx, beadID, beads.UpdateOptions{
			Title:       title,
			Type:        beadType,
			Description: description,
			Status:      status,
		})
		if err != nil {
			return planDataMsg{err: fmt.Errorf("failed to update issue: %w", err)}
		}

		// Refresh after update
		items, err := m.loadBeads()
		session := m.sessionName()
		activeSessions, _ := m.proj.DB.GetBeadsWithActiveSessions(m.ctx, session)
		return planDataMsg{beads: items, activeSessions: activeSessions, err: err}
	}
}

// openInEditor opens the issue in $EDITOR using bd edit
func (m *planModel) openInEditor(beadID string) tea.Cmd {
	beadsPath := m.proj.BeadsPath()

	// Use bd edit which handles $EDITOR and the issue format
	c := beads.EditCommand(m.ctx, beadID, beadsPath)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		if err != nil {
			return planStatusMsg{message: fmt.Sprintf("Editor error: %v", err), isError: true}
		}
		// Refresh data after editing
		return editorFinishedMsg{}
	})
}

// importLinearIssue imports Linear issues (supports multiple IDs/URLs)
func (m *planModel) importLinearIssue(issueIDsInput string) tea.Cmd {
	return func() tea.Msg {
		beadsPath := m.proj.BeadsPath()

		// Get API key from config
		var apiKey string
		if m.proj.Config != nil {
			apiKey = m.proj.Config.Linear.APIKey
		}
		if apiKey == "" {
			return linearImportCompleteMsg{err: fmt.Errorf("linear API key not set (set [linear] api_key in config.toml)")}
		}

		// Create fetcher
		fetcher, err := linear.NewFetcher(apiKey, beadsPath)
		if err != nil {
			return linearImportCompleteMsg{err: fmt.Errorf("failed to create Linear fetcher: %w", err)}
		}

		// Prepare import options from panel
		formResult := m.linearImportPanel.GetResult()
		opts := &linear.ImportOptions{
			DryRun:         formResult.DryRun,
			UpdateExisting: formResult.Update,
			CreateDeps:     formResult.CreateDeps,
			MaxDepDepth:    formResult.MaxDepth,
		}

		// Parse newline-delimited input
		lines := strings.Split(issueIDsInput, "\n")
		var issueIDs []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				issueIDs = append(issueIDs, trimmed)
			}
		}

		// If only one ID, use single import for backward compatibility
		if len(issueIDs) == 1 {
			result, err := fetcher.FetchAndImport(m.ctx, issueIDs[0], opts)
			if err != nil {
				return linearImportCompleteMsg{err: fmt.Errorf("import failed: %w", err)}
			}

			// Check result
			if result.Error != nil {
				return linearImportCompleteMsg{err: result.Error}
			}

			if result.BeadID != "" {
				return linearImportCompleteMsg{
					beadIDs:    []string{result.BeadID},
					skipReason: result.SkipReason,
				}
			}

			return linearImportCompleteMsg{err: fmt.Errorf("import completed but no bead ID returned")}
		}

		// Use batch import for multiple IDs
		results, err := fetcher.FetchBatch(m.ctx, issueIDs, opts)
		if err != nil {
			return linearImportCompleteMsg{err: fmt.Errorf("batch import failed: %w", err)}
		}

		// Collect results
		var beadIDs []string
		var skipReasons []string
		var errors []string
		successCount := 0
		skipCount := 0
		errorCount := 0

		for i, result := range results {
			if result.Error != nil {
				errorCount++
				errors = append(errors, fmt.Sprintf("%s: %v", issueIDs[i], result.Error))
			} else if result.SkipReason != "" {
				skipCount++
				skipReasons = append(skipReasons, fmt.Sprintf("%s: %s", issueIDs[i], result.SkipReason))
			} else if result.BeadID != "" {
				successCount++
				beadIDs = append(beadIDs, result.BeadID)
			}
		}

		// Return aggregated results
		return linearImportCompleteMsg{
			beadIDs:      beadIDs,
			successCount: successCount,
			skipCount:    skipCount,
			errorCount:   errorCount,
			skipReasons:  skipReasons,
			errors:       errors,
		}
	}
}

// prImportCompleteMsg indicates a PR import completed
type prImportCompleteMsg struct {
	workID     string
	prMetadata *github.PRMetadata
	err        error
}

// prImportPreviewMsg indicates a PR preview was fetched
type prImportPreviewMsg struct {
	metadata *github.PRMetadata
	err      error
}

// previewPR fetches PR metadata for preview
func (m *planModel) previewPR(prURL string) tea.Cmd {
	return func() tea.Msg {
		workSvc := work.NewWorkService(m.proj)

		metadata, err := workSvc.GitHubClient.GetPRMetadata(m.ctx, prURL, "")
		if err != nil {
			return prImportPreviewMsg{err: fmt.Errorf("failed to fetch PR: %w", err)}
		}

		return prImportPreviewMsg{metadata: metadata}
	}
}

// importPR imports a PR into a work unit asynchronously via the control plane.
func (m *planModel) importPR(prURL string) tea.Cmd {
	return func() tea.Msg {
		workSvc := work.NewWorkService(m.proj)

		// Fetch PR metadata first
		metadata, err := workSvc.GitHubClient.GetPRMetadata(m.ctx, prURL, "")
		if err != nil {
			return prImportCompleteMsg{err: fmt.Errorf("failed to fetch PR: %w", err)}
		}

		// Use the PR's branch name
		branchName := metadata.HeadRefName

		// Create bead from PR metadata (required for work to function)
		// This is done in the TUI because we need the bead ID before scheduling
		var rootIssueID string
		beadResult, err := workSvc.CreateBeadFromPR(m.ctx, metadata, &work.CreateBeadOptions{
			BeadsDir:     m.proj.BeadsPath(),
			SkipIfExists: true,
		})
		if err != nil {
			return prImportCompleteMsg{err: fmt.Errorf("failed to create bead: %w", err)}
		}
		rootIssueID = beadResult.BeadID

		// Schedule the PR import via the control plane
		result, err := workSvc.ImportPRAsync(m.ctx, work.ImportPRAsyncOptions{
			PRURL:       prURL,
			BranchName:  branchName,
			RootIssueID: rootIssueID,
		})
		if err != nil {
			return prImportCompleteMsg{err: fmt.Errorf("failed to schedule PR import: %w", err)}
		}

		// Ensure control plane is running to process the import task
		if _, err := control.EnsureControlPlane(m.ctx, m.proj); err != nil {
			// Non-fatal: task was scheduled but control plane might need manual start
			return prImportCompleteMsg{
				workID:     result.WorkID,
				prMetadata: metadata,
				err:        fmt.Errorf("import scheduled but control plane failed: %w", err),
			}
		}

		return prImportCompleteMsg{
			workID:     result.WorkID,
			prMetadata: metadata,
		}
	}
}
