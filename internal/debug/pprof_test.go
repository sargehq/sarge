package debug

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStartPprof(t *testing.T) {
	port, err := StartPprof()
	require.NoError(t, err)
	assert.Greater(t, port, 0, "expected a valid port number")

	// Verify pprof endpoint is accessible
	resp, err := http.Get(fmt.Sprintf("http://localhost:%d/debug/pprof/", port))
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}
