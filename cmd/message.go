package cmd

import (
	"fmt"

	"github.com/sargehq/sarge/internal/messages"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var (
	flagMessageWork      string
	flagMessageTask      string
	flagMessageBead      string
	flagMessageEventType string
	flagMessageProject   string
)

var messageCmd = &cobra.Command{
	Use:   "message <text>",
	Short: "[Agent] Send a message to the user via the TUI chat",
	Long: `[Agent Command - Called by Claude Code during prompt processing]

Send a message that appears in the Sarge TUI chat timeline.
This command is for agents spawned by 'sarge' to communicate
results back to the user.

DO NOT use this command from task/implement/review sessions.
It is only for prompt-processing agents spawned from the TUI chat.`,
	Args: cobra.ExactArgs(1),
	RunE: runMessage,
}

func init() {
	messageCmd.Flags().StringVar(&flagMessageWork, "work", "", "associated work ID")
	messageCmd.Flags().StringVar(&flagMessageTask, "task", "", "associated task ID")
	messageCmd.Flags().StringVar(&flagMessageBead, "bead", "", "associated bead ID")
	messageCmd.Flags().StringVar(&flagMessageEventType, "event-type", "", "event type (prompt, work_created, error, info)")
	messageCmd.Flags().StringVar(&flagMessageProject, "project", "", "project directory (default: auto-detect from cwd)")
}

func runMessage(cmd *cobra.Command, args []string) error {
	ctx := GetContext()
	text := args[0]

	proj, err := project.Find(ctx, flagMessageProject)
	if err != nil {
		return fmt.Errorf("not in a project directory: %w", err)
	}
	defer proj.Close()

	err = messages.Write(ctx, proj.DB, messages.WriteParams{
		Source:    messages.SourceSystem,
		Text:      text,
		WorkID:    flagMessageWork,
		TaskID:    flagMessageTask,
		BeadID:    flagMessageBead,
		EventType: flagMessageEventType,
	})
	if err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}

	fmt.Println("Message sent")
	return nil
}
