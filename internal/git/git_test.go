package git_test

import (
	"testing"

	"github.com/sargehq/sarge/internal/git"
	"github.com/stretchr/testify/require"
)

func TestNewOperations(t *testing.T) {
	ops := git.NewOperations()
	require.NotNil(t, ops, "NewOperations returned nil")
}
