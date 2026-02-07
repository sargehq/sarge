package tui

import (
	"os/exec"

	"github.com/sargehq/sarge/internal/tui/components/splash"
)

// buildSplashConfig creates a splash.Config from the current theme and tool checks.
func buildSplashConfig() splash.Config {
	t := CurrentTheme()
	return splash.Config{
		Gradient: t.SplashGradient,
		Accent:   t.Accent,
		Error:    t.Error,
		Warning:  t.Warning,
		Dim:      t.Dim,
		Text:     t.Text,
		Border:   t.DialogBorder,
		HasBd:    isToolAvailable("bd"),
		HasGh:    isToolAvailable("gh"),
		Platform: splash.DetectPlatform(),
	}
}

// isToolAvailable checks if a CLI tool is on PATH.
func isToolAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// shouldShowSplash returns true when the splash screen should be shown:
// no issues, no works, not loading, and initial data fetch completed.
func (m *planModel) shouldShowSplash() bool {
	return m.viewMode == ViewNormal &&
		len(m.beadItems) == 0 &&
		len(m.workTiles) == 0 &&
		!m.loading &&
		!m.lastUpdate.IsZero()
}
