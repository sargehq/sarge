package splash

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// renderToolWarnings renders a centered warning box listing missing tools.
// Returns empty string if all tools are present.
func renderToolWarnings(cfg Config) string {
	if cfg.HasBd && cfg.HasGh {
		return ""
	}

	var sections []string

	if !cfg.HasBd {
		sections = append(sections, renderToolSection(
			"bd (beads) is not installed",
			bdInstallInstructions(),
			cfg,
		))
	}

	if !cfg.HasGh {
		sections = append(sections, renderToolSection(
			"gh (GitHub CLI) is not installed",
			ghInstallInstructions(cfg.Platform),
			cfg,
		))
	}

	body := strings.Join(sections, "\n\n")

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(cfg.Error)

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(cfg.Error).
		Padding(1, 2).
		Width(56)

	title := titleStyle.Render("Missing required tools")
	return boxStyle.Render(title + "\n\n" + body)
}

// renderToolSection renders a single tool's warning section.
func renderToolSection(name, instructions string, cfg Config) string {
	nameStyle := lipgloss.NewStyle().Bold(true).Foreground(cfg.Warning)
	instrStyle := lipgloss.NewStyle().Foreground(cfg.Dim)
	return nameStyle.Render(name) + "\n" + instrStyle.Render(instructions)
}

// bdInstallInstructions returns install instructions for bd.
func bdInstallInstructions() string {
	return "  See https://github.com/beads-project/beads"
}

// ghInstallInstructions returns platform-specific install instructions for gh.
func ghInstallInstructions(platform Platform) string {
	switch platform {
	case PlatformDarwin:
		return "  brew install gh"
	case PlatformLinux:
		return "  Debian/Ubuntu: sudo apt install gh\n" +
			"  Fedora/RHEL:   sudo dnf install gh"
	case PlatformWindows:
		return "  winget install --id GitHub.cli"
	default:
		return "  See https://cli.github.com/manual/installation"
	}
}
