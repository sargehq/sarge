package process_test

import (
	"context"
	"errors"
	"testing"

	"github.com/sargehq/sarge/internal/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsProcessRunningWith(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name      string
		pattern   string
		processes []string
		want      bool
		wantErr   bool
	}{
		{
			name:      "process found",
			pattern:   "myapp",
			processes: []string{"/usr/bin/myapp --flag", "/bin/bash", "/usr/bin/other"},
			want:      true,
			wantErr:   false,
		},
		{
			name:      "process not found",
			pattern:   "nonexistent",
			processes: []string{"/usr/bin/myapp", "/bin/bash"},
			want:      false,
			wantErr:   false,
		},
		{
			name:      "empty process list",
			pattern:   "myapp",
			processes: []string{},
			want:      false,
			wantErr:   false,
		},
		{
			name:      "empty pattern matches all",
			pattern:   "",
			processes: []string{"/usr/bin/myapp"},
			want:      true,
			wantErr:   false,
		},
		{
			name:      "partial match",
			pattern:   "app",
			processes: []string{"/usr/bin/myapp", "/bin/bash"},
			want:      true,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &process.ProcessListerMock{
				GetProcessListFunc: func(ctx context.Context) ([]string, error) {
					return tt.processes, nil
				},
			}
			got, err := process.IsProcessRunningWith(ctx, tt.pattern, lister)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsProcessRunningWith_ListerError(t *testing.T) {
	ctx := context.Background()
	lister := &process.ProcessListerMock{
		GetProcessListFunc: func(ctx context.Context) ([]string, error) {
			return nil, errors.New("ps command failed")
		},
	}

	_, err := process.IsProcessRunningWith(ctx, "myapp", lister)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get process list")
}

func TestKillProcessWith(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name           string
		pattern        string
		processes      []string
		killerErr      error
		wantErr        bool
		wantKillCalled bool
	}{
		{
			name:           "kills matching process",
			pattern:        "myapp",
			processes:      []string{"/usr/bin/myapp --flag", "/bin/bash"},
			wantErr:        false,
			wantKillCalled: true,
		},
		{
			name:           "no matching process - no kill called",
			pattern:        "nonexistent",
			processes:      []string{"/usr/bin/myapp", "/bin/bash"},
			wantErr:        false,
			wantKillCalled: false,
		},
		{
			name:           "empty pattern returns error",
			pattern:        "",
			processes:      []string{"/usr/bin/myapp"},
			wantErr:        true,
			wantKillCalled: false,
		},
		{
			name:           "killer error propagates",
			pattern:        "myapp",
			processes:      []string{"/usr/bin/myapp"},
			killerErr:      errors.New("pkill failed"),
			wantErr:        true,
			wantKillCalled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lister := &process.ProcessListerMock{
				GetProcessListFunc: func(ctx context.Context) ([]string, error) {
					return tt.processes, nil
				},
			}
			var killedPatterns []string
			killer := &process.ProcessKillerMock{
				KillByPatternFunc: func(ctx context.Context, pattern string) error {
					killedPatterns = append(killedPatterns, pattern)
					return tt.killerErr
				},
			}

			err := process.KillProcessWith(ctx, tt.pattern, lister, killer)

			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			if tt.wantKillCalled {
				assert.Contains(t, killedPatterns, tt.pattern)
			} else {
				assert.Empty(t, killedPatterns)
			}
		})
	}
}

func TestKillProcessWith_ListerError(t *testing.T) {
	ctx := context.Background()
	lister := &process.ProcessListerMock{
		GetProcessListFunc: func(ctx context.Context) ([]string, error) {
			return nil, errors.New("ps command failed")
		},
	}
	killer := &process.ProcessKillerMock{}

	err := process.KillProcessWith(ctx, "myapp", lister, killer)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get process list")
	assert.Empty(t, killer.KillByPatternCalls())
}
