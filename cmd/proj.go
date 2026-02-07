package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/sargehq/sarge/internal/mise"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/worktree"
	"github.com/spf13/cobra"
)

var (
	flagForce       bool
	flagProjProject string
)

var projCmd = &cobra.Command{
	Use:   "proj",
	Short: "Manage orchestrator projects",
	Long:  `Manage orchestrator projects with isolated worktrees for each task.`,
}

var projCreateCmd = &cobra.Command{
	Use:   "create <dir> <repo>",
	Short: "Create a new orchestrator project",
	Long: `Create a new orchestrator project at the specified directory.

The repo argument can be:
- A local path (will be symlinked into main/)
- A GitHub URL (will be cloned into main/)

Example:
  sarge proj create ~/myproject ~/my-repo
  sarge proj create ~/myproject https://github.com/user/repo`,
	Args: cobra.ExactArgs(2),
	RunE: runProjCreate,
}

var projDestroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Destroy the current project",
	Long: `Destroy the current project, removing all worktrees and the project directory.

Must be run from within a project directory. Use --force to skip confirmation.`,
	RunE: runProjDestroy,
}

var projStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show project status",
	Long:  `Show project status including configuration, active worktrees, and task states.`,
	RunE:  runProjStatus,
}

func init() {
	projDestroyCmd.Flags().BoolVarP(&flagForce, "force", "f", false, "skip confirmation prompt")
	projDestroyCmd.Flags().StringVar(&flagProjProject, "project", "", "project directory (default: auto-detect from cwd)")
	projStatusCmd.Flags().StringVar(&flagProjProject, "project", "", "project directory (default: auto-detect from cwd)")

	projCmd.AddCommand(projCreateCmd)
	projCmd.AddCommand(projDestroyCmd)
	projCmd.AddCommand(projStatusCmd)
}

func runProjCreate(cmd *cobra.Command, args []string) error {
	dir := args[0]
	repo := args[1]
	ctx := GetContext()

	fmt.Printf("Creating project at %s from %s...\n", dir, repo)

	// Interactive tool selection
	selections := promptToolSelections()

	proj, err := project.CreateWithSelections(ctx, dir, repo, selections)
	if err != nil {
		return fmt.Errorf("failed to create project: %w", err)
	}
	defer proj.Close()

	fmt.Printf("Project '%s' created successfully!\n", proj.Config.Project.Name)
	fmt.Printf("  Directory: %s\n", proj.Root)
	fmt.Printf("  Repo type: %s\n", proj.Config.Repo.Type)
	fmt.Printf("  Repo source: %s\n", proj.Config.Repo.Source)
	fmt.Printf("  Main repo: %s\n", proj.MainRepoPath())

	return nil
}

// isCommandAvailable checks if a command is available on the system PATH.
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// promptYesNo asks the user a yes/no question and returns the answer.
// defaultYes determines the default if the user just presses enter.
func promptYesNo(question string, defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
	suffix := " [Y/n]: "
	if !defaultYes {
		suffix = " [y/N]: "
	}
	fmt.Print(question + suffix)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	if response == "" {
		return defaultYes
	}
	return response == "y" || response == "yes"
}

// promptChoice asks the user to pick from a list of options.
// Returns the selected option string.
func promptChoice(question string, options []string, defaultIdx int) string {
	reader := bufio.NewReader(os.Stdin)
	fmt.Println(question)
	for i, opt := range options {
		marker := "  "
		if i == defaultIdx {
			marker = "* "
		}
		fmt.Printf("  %s%d) %s\n", marker, i+1, opt)
	}
	fmt.Printf("Choice [%d]: ", defaultIdx+1)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(response)
	if response == "" {
		return options[defaultIdx]
	}
	// Parse number
	for i, opt := range options {
		if response == fmt.Sprintf("%d", i+1) {
			return opt
		}
	}
	// Try matching by name
	for _, opt := range options {
		if strings.EqualFold(response, opt) {
			return opt
		}
	}
	return options[defaultIdx]
}

// promptToolSelections interactively asks the user which tools to include.
// It detects already-installed tools and adjusts defaults accordingly.
func promptToolSelections() mise.ToolSelections {
	selections := mise.DefaultToolSelections()

	fmt.Println("\n🔧 Tool Configuration")
	fmt.Println("Configure which tools mise should manage for this project.")
	fmt.Println()

	// Agent type selection
	agentChoice := promptChoice("Which coding agent would you like to use?",
		[]string{"claude", "pi", "none"}, 0)
	selections.AgentType = agentChoice

	// GitHub CLI
	if isCommandAvailable("gh") {
		selections.IncludeGH = promptYesNo("gh (GitHub CLI) is already installed. Include in mise anyway?", false)
	} else {
		selections.IncludeGH = promptYesNo("Include gh (GitHub CLI) in mise?", true)
	}

	// Zellij
	if isCommandAvailable("zellij") {
		selections.IncludeZellij = promptYesNo("zellij is already installed. Include in mise anyway?", false)
	} else {
		selections.IncludeZellij = promptYesNo("Include zellij in mise?", true)
	}

	fmt.Println()
	return selections
}

func runProjDestroy(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	proj, err := project.Find(ctx, flagProjProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// List worktrees
	wtOps := worktree.NewOperations()
	worktrees, err := wtOps.List(ctx, proj.MainRepoPath())
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Filter out the main worktree
	var taskWorktrees []worktree.Worktree
	for _, wt := range worktrees {
		if wt.Path != proj.MainRepoPath() {
			taskWorktrees = append(taskWorktrees, wt)
		}
	}

	// Confirm destruction
	if !flagForce {
		fmt.Printf("About to destroy project '%s' at %s\n", proj.Config.Project.Name, proj.Root)
		if len(taskWorktrees) > 0 {
			fmt.Printf("Active worktrees (%d):\n", len(taskWorktrees))
			for _, wt := range taskWorktrees {
				fmt.Printf("  - %s (%s)\n", wt.Path, wt.Branch)
			}
		}
		fmt.Print("Are you sure? [y/N]: ")

		reader := bufio.NewReader(os.Stdin)
		response, _ := reader.ReadString('\n')
		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" {
			fmt.Println("Aborted.")
			return nil
		}
	}

	// Remove all worktrees
	for _, wt := range taskWorktrees {
		fmt.Printf("Removing worktree %s...\n", wt.Path)
		if err := wtOps.RemoveForce(ctx, proj.MainRepoPath(), wt.Path); err != nil {
			fmt.Printf("Warning: failed to remove worktree %s: %v\n", wt.Path, err)
		}
	}

	// If it's a local symlink, we should not delete the target
	// Just remove the project directory itself
	fmt.Printf("Removing project directory %s...\n", proj.Root)
	if err := os.RemoveAll(proj.Root); err != nil {
		return fmt.Errorf("failed to remove project directory: %w", err)
	}

	fmt.Println("Project destroyed successfully.")
	return nil
}

func runProjStatus(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	proj, err := project.Find(ctx, flagProjProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	// Print project info
	fmt.Printf("Project: %s\n", proj.Config.Project.Name)
	fmt.Printf("Root: %s\n", proj.Root)
	fmt.Printf("Created: %s\n", proj.Config.Project.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("\nRepository:\n")
	fmt.Printf("  Type: %s\n", proj.Config.Repo.Type)
	fmt.Printf("  Source: %s\n", proj.Config.Repo.Source)
	fmt.Printf("  Path: %s\n", proj.MainRepoPath())

	// List worktrees
	wtOps := worktree.NewOperations()
	worktrees, err := wtOps.List(ctx, proj.MainRepoPath())
	if err != nil {
		return fmt.Errorf("failed to list worktrees: %w", err)
	}

	// Filter out the main worktree
	var taskWorktrees []worktree.Worktree
	for _, wt := range worktrees {
		if wt.Path != proj.MainRepoPath() {
			taskWorktrees = append(taskWorktrees, wt)
		}
	}

	fmt.Printf("\nWorktrees: %d\n", len(taskWorktrees))
	if len(taskWorktrees) > 0 {
		for _, wt := range taskWorktrees {
			branch := wt.Branch
			if branch == "" {
				branch = "(detached)"
			}
			fmt.Printf("  - %s [%s]\n", wt.Path, branch)
		}
	}

	// Show task status from database
	beads, err := proj.DB.ListBeads(ctx, "")
	if err != nil {
		fmt.Printf("\nTasks: error listing (%v)\n", err)
		return nil
	}

	if len(beads) > 0 {
		fmt.Printf("\nTracked tasks: %d\n", len(beads))
		for _, b := range beads {
			fmt.Printf("  - %s [%s] %s\n", b.ID, b.Status, b.Title)
		}
	} else {
		fmt.Printf("\nTracked tasks: 0\n")
	}

	return nil
}
