package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/sargehq/sarge/internal/db/sqlc"
	"github.com/sargehq/sarge/internal/logging"
	cosignal "github.com/sargehq/sarge/internal/signal"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migration represents a single migration file
type Migration struct {
	Version string
	Name    string
	UpSQL   string
	DownSQL string
}

// RunMigrations applies all pending migrations from the embedded migrationsFS
func RunMigrations(ctx context.Context, db *sql.DB) error {
	return RunMigrationsForFS(ctx, db, migrationsFS)
}

// RunMigrationsForFS applies all pending migrations from the specified filesystem
func RunMigrationsForFS(ctx context.Context, db *sql.DB, fsys embed.FS) error {
	// Create migrations table if it doesn't exist
	if err := createMigrationsTable(ctx, db); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Get list of applied migrations
	applied, err := getAppliedMigrations(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Read all migration files
	migrations, err := readMigrationsFromFS(fsys)
	if err != nil {
		return fmt.Errorf("failed to read migrations: %w", err)
	}

	// Log what we found
	logging.Debug("Found migration files in filesystem", "count", len(migrations))
	for _, m := range migrations {
		logging.Debug("Migration file", "version", m.Version, "name", m.Name)
	}
	logging.Debug("Found applied migrations in database", "count", len(applied))
	for v := range applied {
		logging.Debug("Applied migration", "version", v)
	}

	// Apply pending migrations with signal blocking to prevent inconsistent state
	for _, m := range migrations {
		if applied[m.Version] {
			logging.Debug("Skipping already applied migration", "version", m.Version)
			continue
		}

		// Block signals during migration to ensure atomicity
		cosignal.BlockSignals()
		fmt.Printf("Applying migration %s: %s\n", m.Version, m.Name)
		if err := applyMigration(ctx, db, m); err != nil {
			cosignal.UnblockSignals()
			return fmt.Errorf("failed to apply migration %s: %w", m.Version, err)
		}
		cosignal.UnblockSignals()
	}

	// Backfill down_sql for any migrations that don't have it yet
	if err := backfillMigrationDownSQL(ctx, db, migrations); err != nil {
		// Log warning but don't fail - backfill is best-effort
		fmt.Printf("Warning: failed to backfill migration down_sql: %v\n", err)
	}

	return nil
}

// RollbackMigration rolls back the last applied migration
func RollbackMigration(ctx context.Context, db *sql.DB) error {
	return RollbackMigrationForFS(ctx, db, migrationsFS)
}

// RollbackMigrationForFS rolls back the last applied migration.
// It first tries to get down_sql from the database (stored when migration was applied).
// Falls back to reading from the filesystem if not available in DB.
func RollbackMigrationForFS(ctx context.Context, db *sql.DB, fsys embed.FS) error {
	queries := sqlc.New(db)

	// Get the last applied migration
	version, err := queries.GetLastMigration(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no migrations to rollback")
	}
	if err != nil {
		return fmt.Errorf("failed to get last migration: %w", err)
	}

	// First, try to get down_sql from database
	var migration *Migration
	dbMigration, err := queries.GetMigrationDownSQL(ctx, version)
	if err == nil && dbMigration.DownSql != "" {
		migration = &Migration{
			Version: version,
			Name:    dbMigration.Name,
			DownSQL: dbMigration.DownSql,
		}
	} else {
		// Fall back to reading from filesystem
		migrations, err := readMigrationsFromFS(fsys)
		if err != nil {
			return fmt.Errorf("failed to read migrations: %w", err)
		}

		for _, m := range migrations {
			if m.Version == version {
				migration = &m
				break
			}
		}
	}

	if migration == nil {
		return fmt.Errorf("migration %s not found in database or filesystem", version)
	}

	if migration.DownSQL == "" {
		return fmt.Errorf("migration %s has no down script", version)
	}

	fmt.Printf("Rolling back migration %s: %s\n", migration.Version, migration.Name)

	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute down statements
	statements := splitSQLStatements(migration.DownSQL)
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to execute statement: %w", err)
		}
	}

	// Remove migration record using sqlc within transaction
	txQueries := queries.WithTx(tx)
	if err := txQueries.DeleteMigration(ctx, version); err != nil {
		return fmt.Errorf("failed to delete migration record: %w", err)
	}

	return tx.Commit()
}

func createMigrationsTable(ctx context.Context, db *sql.DB) error {
	queries := sqlc.New(db)
	return queries.CreateMigrationsTable(ctx)
}

func getAppliedMigrations(ctx context.Context, db *sql.DB) (map[string]bool, error) {
	queries := sqlc.New(db)
	versions, err := queries.GetAppliedMigrations(ctx)
	if err != nil {
		return nil, err
	}

	applied := make(map[string]bool)
	for _, version := range versions {
		applied[version] = true
	}
	return applied, nil
}

func readMigrationsFromFS(fsys embed.FS) ([]Migration, error) {
	var migrations []Migration

	// Walk the entire embedded filesystem to find SQL files
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if d.IsDir() || !strings.HasSuffix(path, ".sql") {
			return nil
		}

		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", path, err)
		}

		// Extract just the filename from the path
		filename := path
		if idx := strings.LastIndex(path, "/"); idx >= 0 {
			filename = path[idx+1:]
		}

		// Parse version and name from filename (e.g., "001_initial.sql")
		parts := strings.SplitN(strings.TrimSuffix(filename, ".sql"), "_", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid migration filename: %s", filename)
		}

		migration := Migration{
			Version: parts[0],
			Name:    parts[1],
			UpSQL:   parseUpSection(string(content)),
			DownSQL: parseDownSection(string(content)),
		}
		migrations = append(migrations, migration)
		return nil
	})

	if err != nil {
		return nil, err
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

func parseUpSection(content string) string {
	lines := strings.Split(content, "\n")
	var upLines []string
	inUp := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "-- +up") {
			inUp = true
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "-- +down") {
			break
		}
		if inUp {
			upLines = append(upLines, line)
		}
	}

	return strings.Join(upLines, "\n")
}

func parseDownSection(content string) string {
	lines := strings.Split(content, "\n")
	var downLines []string
	inDown := false

	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "-- +down") {
			inDown = true
			continue
		}
		if inDown {
			downLines = append(downLines, line)
		}
	}

	return strings.Join(downLines, "\n")
}

func applyMigration(ctx context.Context, db *sql.DB, m Migration) error {
	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Execute up statements
	statements := splitSQLStatements(m.UpSQL)
	logging.Debug("Applying migration", "version", m.Version, "upSqlBytes", len(m.UpSQL), "statementCount", len(statements))
	for i, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			logging.Debug("Skipping empty statement", "version", m.Version, "index", i)
			continue
		}
		logging.Debug("Executing statement", "version", m.Version, "index", i, "sql", stmt)
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("failed to execute statement %d: %w", i, err)
		}
		logging.Debug("Statement succeeded", "version", m.Version, "index", i)
	}

	// Record migration (just version) using sqlc within transaction
	// The backfill step will add name and down_sql after migration 012 adds those columns
	txQueries := sqlc.New(tx)
	logging.Debug("Recording migration in schema_migrations", "version", m.Version)
	if err := txQueries.RecordMigration(ctx, m.Version); err != nil {
		return fmt.Errorf("failed to record migration: %w", err)
	}

	logging.Debug("Committing transaction", "version", m.Version)
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}
	logging.Debug("Transaction committed successfully", "version", m.Version)
	return nil
}

// splitSQLStatements splits SQL content into individual statements
// It properly handles semicolons within strings and comments
func splitSQLStatements(sql string) []string {
	var statements []string
	var current strings.Builder
	inString := false
	var stringChar rune
	inLineComment := false
	inBlockComment := false

	runes := []rune(sql)
	for i := 0; i < len(runes); i++ {
		char := runes[i]
		next := rune(0)
		if i+1 < len(runes) {
			next = runes[i+1]
		}

		// Handle line comments
		if !inString && !inBlockComment && char == '-' && next == '-' {
			inLineComment = true
			current.WriteRune(char)
			continue
		}
		if inLineComment {
			current.WriteRune(char)
			if char == '\n' {
				inLineComment = false
			}
			continue
		}

		// Handle block comments
		if !inString && !inLineComment && char == '/' && next == '*' {
			inBlockComment = true
			current.WriteRune(char)
			continue
		}
		if inBlockComment {
			current.WriteRune(char)
			if char == '*' && next == '/' {
				current.WriteRune(next)
				i++ // Skip the next character
				inBlockComment = false
			}
			continue
		}

		// Handle strings
		if !inLineComment && !inBlockComment {
			if !inString && (char == '\'' || char == '"' || char == '`') {
				inString = true
				stringChar = char
			} else if inString && char == stringChar {
				// Check if it's escaped
				escaped := false
				for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
					escaped = !escaped
				}
				if !escaped {
					inString = false
				}
			}
		}

		// Handle statement separator
		if !inString && !inLineComment && !inBlockComment && char == ';' {
			stmt := strings.TrimSpace(current.String())
			if stmt != "" {
				statements = append(statements, stmt)
			}
			current.Reset()
			continue
		}

		current.WriteRune(char)
	}

	// Add any remaining statement
	if stmt := strings.TrimSpace(current.String()); stmt != "" {
		statements = append(statements, stmt)
	}

	return statements
}

// MigrationStatusContext returns the current migration status
func MigrationStatusContext(ctx context.Context, db *sql.DB) ([]string, error) {
	queries := sqlc.New(db)
	versions, err := queries.ListMigrationVersions(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			return nil, nil // No migrations applied yet
		}
		return nil, err
	}
	return versions, nil
}

// backfillMigrationDownSQL updates existing migration records with their down_sql.
// This is called after applying migrations to ensure all records have down_sql populated.
func backfillMigrationDownSQL(ctx context.Context, db *sql.DB, migrations []Migration) error {
	queries := sqlc.New(db)

	// Get all applied migrations with their current down_sql
	appliedList, err := queries.ListMigrationsWithDetails(ctx)
	if err != nil {
		return fmt.Errorf("failed to list migrations: %w", err)
	}

	// Create a map for quick lookup
	migrationMap := make(map[string]Migration)
	for _, m := range migrations {
		migrationMap[m.Version] = m
	}

	// Update any migrations that have empty down_sql
	for _, applied := range appliedList {
		if applied.DownSql != "" {
			continue // Already has down_sql
		}

		m, ok := migrationMap[applied.Version]
		if !ok {
			continue // Migration file not found (shouldn't happen)
		}

		if m.DownSQL == "" {
			continue // No down_sql in file
		}

		// Update the record with down_sql from the file
		if err := queries.UpdateMigrationDownSQL(ctx, sqlc.UpdateMigrationDownSQLParams{
			Name:    m.Name,
			DownSql: m.DownSQL,
			Version: m.Version,
		}); err != nil {
			return fmt.Errorf("failed to update migration %s: %w", m.Version, err)
		}
	}

	return nil
}
