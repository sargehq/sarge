package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestZoneMarking verifies that panels create proper zone prefixes for bubblezone
func TestZoneMarking(t *testing.T) {
	t.Run("status bar has zone prefix", func(t *testing.T) {
		sb := NewStatusBar()
		assert.NotEmpty(t, sb.zonePrefix, "StatusBar should have a zone prefix")
	})

	t.Run("issues panel has zone prefix", func(t *testing.T) {
		ip := NewIssuesPanel()
		assert.NotEmpty(t, ip.zonePrefix, "IssuesPanel should have a zone prefix")
	})

	t.Run("work overview panel has zone prefix", func(t *testing.T) {
		wop := NewWorkOverviewPanel()
		assert.NotEmpty(t, wop.zonePrefix, "WorkOverviewPanel should have a zone prefix")
	})
}
