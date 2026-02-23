package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sargehq/sarge/internal/beans"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/github"
	"github.com/sargehq/sarge/internal/linear"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/work"
)

// refreshData creates a tea.Cmd that refreshes bean data
func (m *planModel) refreshData() tea.Cmd {
	// Capture current filter and sequence at creation time to avoid race conditions
	filters := m.filters
	seq := m.searchSeq
	return m.refreshDataWithFilters(filters, seq)
}

// refreshDataWithFilters creates a refresh command with captured filter values.
// This prevents race conditions when the user types quickly.
func (m *planModel) refreshDataWithFilters(filters beanFilters, seq uint64) tea.Cmd {
	return func() tea.Msg {
		items, err := m.loadBeansWithFilters(filters)

		// Also fetch active sessions
		session := m.sessionName()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)

		return planDataMsg{
			beans:          items,
			activeSessions: activeSessions,
			err:            err,
			searchSeq:      seq,
		}
	}
}

func (m *planModel) loadBeans() ([]beanItem, error) {
	return m.loadBeansWithFilters(m.filters)
}

// loadBeansWithFilters loads beans using the provided filters.
// This allows capturing filters at command creation time to avoid race conditions.
func (m *planModel) loadBeansWithFilters(filters beanFilters) ([]beanItem, error) {
	mainRepoPath := m.proj.MainRepoPath()

	// Handle task filter - show beans assigned to a specific task
	if filters.task != "" {
		return m.loadBeansForTask(filters)
	}

	// Handle children filter - show children (dependents) of a specific bean
	if filters.children != "" {
		return m.loadBeansForChildren(filters)
	}

	// Use the shared fetchBeansWithFilters function
	items, err := fetchBeansWithFilters(m.ctx, m.proj.Beans, mainRepoPath, filters)
	if err != nil {
		return nil, err
	}

	// Ensure root issue is included in results when set (e.g., when work panel is showing
	// but user pressed '*' to clear task/children filter)
	if filters.rootIssue != "" {
		found := false
		for _, item := range items {
			if item.ID == filters.rootIssue {
				found = true
				break
			}
		}
		if !found {
			rootBean, err := m.proj.Beans.GetBean(m.ctx, filters.rootIssue)
			if err == nil && rootBean != nil {
				items = append([]beanItem{{BeanWithDeps: rootBean}}, items...)
			}
		}
	}

	// Fetch assigned beans from database and populate assignedWorkID
	assignedBeans, err := m.proj.DB.GetAllAssignedBeans(m.ctx)
	if err == nil {
		for i := range items {
			if workID, ok := assignedBeans[items[i].ID]; ok {
				items[i].assignedWorkID = workID
			}
		}
	}

	// Build tree structure from dependencies
	// Preserve root issue so it's never filtered out by the closed-item visibility filter
	items = buildBeanTree(m.ctx, items, m.proj.Beans, filters.rootIssue)

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

// loadBeansForTask loads beans assigned to a specific task.
// This fetches all beans for the task regardless of status filter.
// If filters.rootIssue is set, the root issue is prepended to the results.
func (m *planModel) loadBeansForTask(filters beanFilters) ([]beanItem, error) {
	// Get bean IDs assigned to this task from the database
	beanIDs, err := m.proj.DB.GetTaskBeans(m.ctx, filters.task)
	if err != nil {
		return nil, fmt.Errorf("failed to get task beans: %w", err)
	}

	// Track which IDs we have to avoid duplicates
	beanIDSet := make(map[string]bool)
	for _, id := range beanIDs {
		beanIDSet[id] = true
	}

	// Start with root issue if specified and not already in the task
	var items []beanItem
	if filters.rootIssue != "" && !beanIDSet[filters.rootIssue] {
		rootBean, err := m.proj.Beans.GetBean(m.ctx, filters.rootIssue)
		if err == nil && rootBean != nil {
			items = append(items, beanItem{
				BeanWithDeps: rootBean,
			})
		}
	}

	// Fetch the beans from the beans client (uses cache)
	for _, beanID := range beanIDs {
		bean, err := m.proj.Beans.GetBean(m.ctx, beanID)
		if err != nil || bean == nil {
			continue
		}
		items = append(items, beanItem{
			BeanWithDeps: bean,
		})
	}

	if len(items) == 0 {
		return nil, nil
	}

	// Apply search text filter if set
	if filters.searchText != "" {
		searchLower := strings.ToLower(filters.searchText)
		var filtered []beanItem
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.ID), searchLower) ||
				strings.Contains(strings.ToLower(item.Title), searchLower) ||
				strings.Contains(strings.ToLower(item.Body), searchLower) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// Build tree structure from dependencies
	// Preserve root issue so it's never filtered out by the closed-item visibility filter
	items = buildBeanTree(m.ctx, items, m.proj.Beans, filters.rootIssue)

	return items, nil
}

// loadBeansForChildren loads children (dependents) of a specific bean.
// This fetches all dependents regardless of status filter.
func (m *planModel) loadBeansForChildren(filters beanFilters) ([]beanItem, error) {
	// Get the parent bean to find its dependents
	parentBean, err := m.proj.Beans.GetBean(m.ctx, filters.children)
	if err != nil {
		return nil, fmt.Errorf("failed to get bean: %w", err)
	}
	if parentBean == nil {
		return nil, nil
	}

	// Also include the parent bean itself
	items := []beanItem{{BeanWithDeps: parentBean}}

	// Fetch each dependent bean
	for _, dep := range parentBean.Dependents {
		bean, err := m.proj.Beans.GetBean(m.ctx, dep.BeanID)
		if err != nil || bean == nil {
			continue
		}
		items = append(items, beanItem{
			BeanWithDeps: bean,
		})
	}

	// Apply search text filter if set
	if filters.searchText != "" {
		searchLower := strings.ToLower(filters.searchText)
		var filtered []beanItem
		for _, item := range items {
			if strings.Contains(strings.ToLower(item.ID), searchLower) ||
				strings.Contains(strings.ToLower(item.Title), searchLower) ||
				strings.Contains(strings.ToLower(item.Body), searchLower) {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}

	// Build tree structure from dependencies
	// Preserve the parent (root issue) so it's never filtered out by the closed-item visibility filter
	items = buildBeanTree(m.ctx, items, m.proj.Beans, filters.children)

	return items, nil
}

func (m *planModel) createBean(title, beanType string, priority string, isEpic bool, description string, parent string) tea.Cmd {
	return func() tea.Msg {
		ctx := m.ctx
		beansPath := m.proj.BeansPath()

		beanID, err := beans.NewCLI(beansPath).Create(ctx, beans.CreateOptions{
			Title:    title,
			Type:     beanType,
			Priority: priority,
			IsEpic:   isEpic,
			Body:     description,
			Parent:   parent,
		})
		if err != nil {
			return planDataMsg{err: fmt.Errorf("failed to create issue: %w", err)}
		}

		// Refresh after creation
		items, err := m.loadBeans()
		session := m.sessionName()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)

		return planDataMsg{beans: items, activeSessions: activeSessions, err: err, createdBeanID: beanID}
	}
}

func (m *planModel) closeBean(beanID string) tea.Cmd {
	return func() tea.Msg {
		beansPath := m.proj.BeansPath()
		session := m.sessionName()
		tabName := db.TabNameForBean(beanID)

		// If there's an active session for this bean, close it
		if m.activeBeanSessions[beanID] {
			// Terminate and close the tab
			_ = m.zj.Session(session).TerminateAndCloseTab(m.ctx, tabName)
			// Unregister from database
			_ = m.proj.DB.UnregisterPlanSession(m.ctx, beanID)
		}

		// Close the bean
		if err := beans.NewCLI(beansPath).Close(m.ctx, beanID); err != nil {
			return planDataMsg{err: fmt.Errorf("failed to close issue: %w", err)}
		}

		// Refresh after close
		items, err := m.loadBeans()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)
		return planDataMsg{beans: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) closeBeans(beanIDs []string) tea.Cmd {
	return func() tea.Msg {
		beansPath := m.proj.BeansPath()
		session := m.sessionName()

		// First, close any active sessions for these beans
		zjSession := m.zj.Session(session)
		for _, beanID := range beanIDs {
			if m.activeBeanSessions[beanID] {
				tabName := db.TabNameForBean(beanID)
				// Terminate and close the tab
				_ = zjSession.TerminateAndCloseTab(m.ctx, tabName)
				// Unregister from database
				_ = m.proj.DB.UnregisterPlanSession(m.ctx, beanID)
			}
		}

		// Close all beans using the beans package
		cli := beans.NewCLI(beansPath)
		for _, beanID := range beanIDs {
			if err := cli.Close(m.ctx, beanID); err != nil {
				return planDataMsg{err: fmt.Errorf("failed to close issue %s: %w", beanID, err)}
			}
		}

		// Refresh after close
		items, err := m.loadBeans()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)
		return planDataMsg{beans: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) deleteBean(beanID string) tea.Cmd {
	return func() tea.Msg {
		beansPath := m.proj.BeansPath()
		session := m.sessionName()
		tabName := db.TabNameForBean(beanID)

		// If there's an active session for this bean, close it
		if m.activeBeanSessions[beanID] {
			// Terminate and close the tab
			_ = m.zj.Session(session).TerminateAndCloseTab(m.ctx, tabName)
			// Unregister from database
			_ = m.proj.DB.UnregisterPlanSession(m.ctx, beanID)
		}

		// Delete the bean permanently with --force to skip confirmation
		if err := beans.NewCLI(beansPath).Delete(m.ctx, beanID, true); err != nil {
			return planDataMsg{err: fmt.Errorf("failed to delete issue: %w", err)}
		}

		// Refresh after delete
		items, err := m.loadBeans()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)
		return planDataMsg{beans: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) deleteBeans(beanIDs []string) tea.Cmd {
	return func() tea.Msg {
		beansPath := m.proj.BeansPath()
		session := m.sessionName()

		// First, close any active sessions for these beans
		zjSession := m.zj.Session(session)
		for _, beanID := range beanIDs {
			if m.activeBeanSessions[beanID] {
				tabName := db.TabNameForBean(beanID)
				// Terminate and close the tab
				_ = zjSession.TerminateAndCloseTab(m.ctx, tabName)
				// Unregister from database
				_ = m.proj.DB.UnregisterPlanSession(m.ctx, beanID)
			}
		}

		// Delete all beans permanently with --force
		cli := beans.NewCLI(beansPath)
		for _, beanID := range beanIDs {
			if err := cli.Delete(m.ctx, beanID, true); err != nil {
				return planDataMsg{err: fmt.Errorf("failed to delete issue %s: %w", beanID, err)}
			}
		}

		// Refresh after delete
		items, err := m.loadBeans()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)
		return planDataMsg{beans: items, activeSessions: activeSessions, err: err}
	}
}

func (m *planModel) saveBeanEdit(beanID, title, description, beanType, status string) tea.Cmd {
	return func() tea.Msg {
		beansPath := m.proj.BeansPath()

		// Update the bean using beans package
		err := beans.NewCLI(beansPath).Update(m.ctx, beanID, beans.UpdateOptions{
			Title:  title,
			Type:   beanType,
			Body:   description,
			Status: status,
		})
		if err != nil {
			return planDataMsg{err: fmt.Errorf("failed to update issue: %w", err)}
		}

		// Refresh after update
		items, err := m.loadBeans()
		session := m.sessionName()
		activeSessions, _ := m.proj.DB.GetBeansWithActiveSessions(m.ctx, session)
		return planDataMsg{beans: items, activeSessions: activeSessions, err: err}
	}
}

// openInEditor opens the issue in $EDITOR using bd edit
func (m *planModel) openInEditor(beanID string) tea.Cmd {
	beansPath := m.proj.BeansPath()

	// Use bd edit which handles $EDITOR and the issue format
	c := beans.EditCommand(m.ctx, beanID, beansPath)
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
		beansPath := m.proj.BeansPath()

		// Get API key from config
		var apiKey string
		if m.proj.Config != nil {
			apiKey = m.proj.Config.Linear.APIKey
		}
		if apiKey == "" {
			return linearImportCompleteMsg{err: fmt.Errorf("linear API key not set (set [linear] api_key in config.toml)")}
		}

		// Create fetcher
		fetcher, err := linear.NewFetcher(apiKey, beansPath)
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

			if result.BeanID != "" {
				return linearImportCompleteMsg{
					beanIDs:    []string{result.BeanID},
					skipReason: result.SkipReason,
				}
			}

			return linearImportCompleteMsg{err: fmt.Errorf("import completed but no bean ID returned")}
		}

		// Use batch import for multiple IDs
		results, err := fetcher.FetchBatch(m.ctx, issueIDs, opts)
		if err != nil {
			return linearImportCompleteMsg{err: fmt.Errorf("batch import failed: %w", err)}
		}

		// Collect results
		var beanIDs []string
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
			} else if result.BeanID != "" {
				successCount++
				beanIDs = append(beanIDs, result.BeanID)
			}
		}

		// Return aggregated results
		return linearImportCompleteMsg{
			beanIDs:      beanIDs,
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

		// Create bean from PR metadata (required for work to function)
		// This is done in the TUI because we need the bean ID before scheduling
		var rootIssueID string
		beanResult, err := workSvc.CreateBeanFromPR(m.ctx, metadata, &work.CreateBeanOptions{
			BeansDir:     m.proj.BeansPath(),
			SkipIfExists: true,
		})
		if err != nil {
			return prImportCompleteMsg{err: fmt.Errorf("failed to create bean: %w", err)}
		}
		rootIssueID = beanResult.BeanID

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
