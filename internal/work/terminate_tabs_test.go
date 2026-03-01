package work

import (
	"bytes"
	"context"
	"testing"

	"github.com/sargehq/sarge/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTerminateWorkTabs_OrchestratorProcessNotFound(t *testing.T) {
	testDB, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	mgr := &DefaultOrchestratorManager{
		database: testDB,
	}

	ctx := context.Background()
	var buf bytes.Buffer
	err = mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)
}

func TestTerminateWorkTabs_NoBridge(t *testing.T) {
	testDB, err := db.OpenPath(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { testDB.Close() })

	mgr := &DefaultOrchestratorManager{
		database: testDB,
	}

	ctx := context.Background()
	var buf bytes.Buffer
	err = mgr.TerminateWorkTabs(ctx, "w-abc", "test-project", &buf)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func strPtr(s string) *string {
	return &s
}
