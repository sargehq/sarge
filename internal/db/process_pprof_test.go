package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdatePprofPort(t *testing.T) {
	database, cleanup := setupTestDB(t)
	defer cleanup()
	ctx := context.Background()

	// Register a process first
	err := database.RegisterProcess(ctx, "test-proc", ProcessTypeControlPlane, nil, 12345)
	require.NoError(t, err)

	// Set pprof port
	port := 9090
	err = database.UpdatePprofPort(ctx, "test-proc", &port)
	require.NoError(t, err)

	// Verify port was stored
	proc, err := database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)
	require.NotNil(t, proc.PprofPort)
	assert.Equal(t, 9090, *proc.PprofPort)

	// Clear pprof port
	err = database.UpdatePprofPort(ctx, "test-proc", nil)
	require.NoError(t, err)

	proc, err = database.GetControlPlaneProcess(ctx)
	require.NoError(t, err)
	require.NotNil(t, proc)
	assert.Nil(t, proc.PprofPort, "pprof port should be nil after clearing")
}
