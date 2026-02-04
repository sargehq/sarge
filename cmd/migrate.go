package cmd

import (
	"fmt"

	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/project"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Manage database migrations",
	Long:  `Manage database migrations for the co tracking database.`,
}

var migrateStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show applied migrations",
	Long:  `Show a list of all applied database migrations.`,
	Args:  cobra.NoArgs,
	RunE:  runMigrateStatus,
}

var migrateUpCmd = &cobra.Command{
	Use:   "up",
	Short: "Apply pending migrations",
	Long:  `Apply all pending database migrations. This happens automatically when the database is accessed, but can be run manually if needed.`,
	Args:  cobra.NoArgs,
	RunE:  runMigrateUp,
}

var migrateRollbackCmd = &cobra.Command{
	Use:   "rollback",
	Short: "Rollback the last migration",
	Long:  `Rollback the most recently applied database migration.`,
	Args:  cobra.NoArgs,
	RunE:  runMigrateRollback,
}

func init() {
	migrateCmd.AddCommand(migrateStatusCmd)
	migrateCmd.AddCommand(migrateUpCmd)
	migrateCmd.AddCommand(migrateRollbackCmd)
	rootCmd.AddCommand(migrateCmd)
}

func runMigrateStatus(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Get migration status
	versions, err := db.MigrationStatusContext(ctx, proj.DB.DB)
	if err != nil {
		return fmt.Errorf("failed to get migration status: %w", err)
	}

	if len(versions) == 0 {
		fmt.Println("No migrations applied.")
		return nil
	}

	fmt.Printf("Applied migrations (%d):\n", len(versions))
	for _, version := range versions {
		fmt.Printf("  %s\n", version)
	}

	return nil
}

func runMigrateUp(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Run migrations (they're already run on project open, but we can run again to ensure latest)
	if err := db.RunMigrations(ctx, proj.DB.DB); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	fmt.Println("All migrations applied successfully.")
	return nil
}

func runMigrateRollback(cmd *cobra.Command, args []string) error {
	ctx := GetContext()

	// Find project
	proj, err := project.Find(ctx, "")
	if err != nil {
		return err
	}
	defer proj.Close()

	// Rollback last migration
	if err := db.RollbackMigration(ctx, proj.DB.DB); err != nil {
		return fmt.Errorf("failed to rollback migration: %w", err)
	}

	fmt.Println("Migration rolled back successfully.")
	return nil
}
