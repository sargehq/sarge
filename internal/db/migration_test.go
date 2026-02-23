package db

import (
	"context"
	"database/sql"
	"embed"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
)

//go:embed test_migrations/*.sql
var testMigrationsFS embed.FS

// hasColumn checks if a table has a specific column
//
//nolint:unparam // table parameter allows different table names in future tests
func hasColumn(ctx context.Context, db *sql.DB, table, column string) (bool, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue any
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// TestRunMigrations tests the basic migration functionality
func TestRunMigrations(t *testing.T) {
	// Create an in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	ctx := context.Background()

	// Run migrations with test filesystem
	err = RunMigrationsForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to run migrations")

	// Verify schema_migrations table exists
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'").Scan(&count)
	require.NoError(t, err, "Failed to check for schema_migrations table")
	assert.Equal(t, 1, count, "Expected schema_migrations table to exist")

	// Verify migrations were recorded
	versions, err := MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status")
	assert.Len(t, versions, 2, "Expected 2 migrations to be recorded")

	// Verify test tables were created (from test migrations)
	tables := []string{"test_table1", "test_table2"}
	for _, table := range tables {
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table).Scan(&count)
		require.NoError(t, err, "Failed to check for table %s", table)
		assert.Equal(t, 1, count, "Expected table %s to exist", table)
	}

	// Verify index was created
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_test_table1_name'").Scan(&count)
	require.NoError(t, err, "Failed to check for index")
	assert.Equal(t, 1, count, "Expected index idx_test_table1_name to exist")
}

// TestRunMigrationsIdempotent tests that running migrations multiple times is safe
func TestRunMigrationsIdempotent(t *testing.T) {
	// Create an in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	// Run migrations first time
	ctx := context.Background()
	err = RunMigrationsForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to run migrations first time")

	// Run migrations second time - should be idempotent
	err = RunMigrationsForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to run migrations second time")

	// Verify only one migration record exists
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE version = '001'").Scan(&count)
	require.NoError(t, err, "Failed to count migration records")
	assert.Equal(t, 1, count, "Expected 1 migration record")
}

// TestMigrationStatus tests the MigrationStatus function
func TestMigrationStatusContext(t *testing.T) {
	// Create an in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to open database")
	defer db.Close()
	ctx := context.Background()

	// Check status before migrations
	versions, err := MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status")
	assert.Empty(t, versions, "Expected no migrations before running")

	// Run migrations
	err = RunMigrationsForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to run migrations")

	// Check status after migrations
	versions, err = MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status")
	require.Len(t, versions, 2, "Expected 2 migrations")
	assert.Equal(t, "001", versions[0], "Expected first migration 001")
	assert.Equal(t, "002", versions[1], "Expected second migration 002")
}

// TestSplitSQLStatements tests the SQL statement splitter
func TestSplitSQLStatements(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:  "Simple statements",
			input: "CREATE TABLE t1 (id INT);\nCREATE TABLE t2 (id INT);",
			expected: []string{
				"CREATE TABLE t1 (id INT)",
				"CREATE TABLE t2 (id INT)",
			},
		},
		{
			name:  "Statement with string containing semicolon",
			input: `INSERT INTO t1 VALUES ('test;value'); CREATE TABLE t2 (id INT);`,
			expected: []string{
				`INSERT INTO t1 VALUES ('test;value')`,
				"CREATE TABLE t2 (id INT)",
			},
		},
		{
			name:  "Statement with line comment",
			input: "-- This is a comment\nCREATE TABLE t1 (id INT);",
			expected: []string{
				"-- This is a comment\nCREATE TABLE t1 (id INT)",
			},
		},
		{
			name:  "Statement with block comment",
			input: "/* Multi\n   line\n   comment */\nCREATE TABLE t1 (id INT);",
			expected: []string{
				"/* Multi\n   line\n   comment */\nCREATE TABLE t1 (id INT)",
			},
		},
		{
			name: "Multiple statements with various features",
			input: `CREATE TABLE users (
				id INT PRIMARY KEY,
				name TEXT DEFAULT 'John;Doe'
			);
			-- Create index
			CREATE INDEX idx_name ON users(name);
			/* Another table */
			CREATE TABLE posts (
				id INT,
				content TEXT
			);`,
			expected: []string{
				`CREATE TABLE users (
				id INT PRIMARY KEY,
				name TEXT DEFAULT 'John;Doe'
			)`,
				`-- Create index
			CREATE INDEX idx_name ON users(name)`,
				`/* Another table */
			CREATE TABLE posts (
				id INT,
				content TEXT
			)`,
			},
		},
		{
			name:     "Empty input",
			input:    "",
			expected: []string{},
		},
		{
			name:     "Only whitespace and comments",
			input:    "  \n-- comment\n  ",
			expected: []string{"-- comment"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := splitSQLStatements(tt.input)
			require.Len(t, result, len(tt.expected), "Statement count mismatch")
			for i, stmt := range result {
				assert.Equal(t, tt.expected[i], stmt, "Statement %d mismatch", i)
			}
		})
	}
}

// TestParseUpDownSections tests parsing of up and down sections
func TestParseUpDownSections(t *testing.T) {
	migration := `-- Initial migration
-- +up
CREATE TABLE users (
    id INT PRIMARY KEY,
    name TEXT
);
CREATE INDEX idx_users_name ON users(name);

-- +down
DROP TABLE users;
`

	upSQL := parseUpSection(migration)
	expectedUp := `CREATE TABLE users (
    id INT PRIMARY KEY,
    name TEXT
);
CREATE INDEX idx_users_name ON users(name);
`
	assert.Equal(t, expectedUp, upSQL, "Up section mismatch")

	downSQL := parseDownSection(migration)
	expectedDown := "DROP TABLE users;\n"
	assert.Equal(t, expectedDown, downSQL, "Down section mismatch")
}

// TestOpenPath tests the OpenPath function which integrates migration running
func TestOpenPath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	ctx := context.Background()

	// Open database (should run migrations)
	db, err := OpenPath(ctx, dbPath)
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	// Verify migrations were run by checking for tables
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='beans'").Scan(&count)
	require.NoError(t, err, "Failed to check for beans table")
	assert.Equal(t, 1, count, "Expected beans table to exist after OpenPath")

	// Verify we can use the queries
	assert.NotNil(t, db.queries, "Expected queries to be initialized")
}

// TestRollbackMigration tests the rollback functionality
func TestRollbackMigration(t *testing.T) {
	// Create an in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	ctx := context.Background()

	// Run migrations (runs both 001 and 002)
	err = RunMigrationsForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to run migrations")

	// Verify we have 2 migrations
	versions, err := MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status")
	assert.Len(t, versions, 2, "Expected 2 migrations before rollback")

	// Verify the test_field column exists (from migration 002)
	hasTestField, err := hasColumn(ctx, db, "test_table1", "test_field")
	require.NoError(t, err, "Failed to check for test_field column")
	assert.True(t, hasTestField, "Expected test_field column to exist before rollback")

	// Rollback the latest migration (002)
	err = RollbackMigrationForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to rollback migration")

	// Verify we now have 1 migration left (001)
	versions, err = MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status after rollback")
	require.Len(t, versions, 1, "Expected 1 migration after rollback")
	assert.Equal(t, "001", versions[0], "Expected migration 001 to remain")

	// Verify test_table1 still exists (from migration 001)
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test_table1'").Scan(&count)
	require.NoError(t, err, "Failed to check for test_table1 after rollback")
	assert.Equal(t, 1, count, "Expected test_table1 to still exist after rolling back 002")

	// Verify test_field column is gone (from migration 002)
	hasTestField, err = hasColumn(ctx, db, "test_table1", "test_field")
	require.NoError(t, err, "Failed to check for test_field column after rollback")
	assert.False(t, hasTestField, "Expected test_field column to be gone after rollback")

	// Rollback the remaining migration (001)
	err = RollbackMigrationForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to rollback migration 001")

	// Verify tables are gone after rolling back all migrations
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='test_table1'").Scan(&count)
	require.NoError(t, err, "Failed to check for test_table1 after final rollback")
	assert.Equal(t, 0, count, "Expected test_table1 to be gone after rolling back all migrations")

	// Verify no migration records remain
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count)
	require.NoError(t, err, "Failed to check migration records")
	assert.Equal(t, 0, count, "Expected no migration records after rolling back all")
}

// TestRollbackNoMigrations tests rollback when there are no migrations
func TestRollbackNoMigrations(t *testing.T) {
	// Create an in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	ctx := context.Background()

	// Create migrations table but don't run migrations
	err = createMigrationsTable(ctx, db)
	require.NoError(t, err, "Failed to create migrations table")

	// Try to rollback when there are no migrations
	err = RollbackMigrationForFS(ctx, db, testMigrationsFS)
	require.Error(t, err, "Expected error when rolling back with no migrations")
	assert.Equal(t, "no migrations to rollback", err.Error())
}

// TestMultipleMigrations tests running multiple migrations in sequence
func TestMultipleMigrations(t *testing.T) {
	// Create an in-memory database
	db, err := sql.Open("sqlite3", ":memory:")
	require.NoError(t, err, "Failed to open database")
	defer db.Close()

	ctx := context.Background()

	// Run migrations (should run both 001 and 002)
	err = RunMigrationsForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to run migrations")

	// Check that both migrations were applied
	versions, err := MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status")
	assert.Len(t, versions, 2, "Expected 2 migrations")

	// Verify the test_field column was added
	hasTestField, err := hasColumn(ctx, db, "test_table1", "test_field")
	require.NoError(t, err, "Failed to check for test_field column")
	assert.True(t, hasTestField, "Expected test_field column to exist after migration 002")

	// Rollback second migration
	err = RollbackMigrationForFS(ctx, db, testMigrationsFS)
	require.NoError(t, err, "Failed to rollback migration")

	// Verify only first migration remains
	versions, err = MigrationStatusContext(ctx, db)
	require.NoError(t, err, "Failed to get migration status after rollback")
	require.Len(t, versions, 1, "Expected 1 migration after rollback")
	assert.Equal(t, "001", versions[0], "Expected migration 001 to remain")

	// Verify test_field column was removed
	hasTestField, err = hasColumn(ctx, db, "test_table1", "test_field")
	require.NoError(t, err, "Failed to check for test_field column after rollback")
	assert.False(t, hasTestField, "Expected test_field column to be removed after rollback")
}
