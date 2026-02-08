package tui

import "github.com/charmbracelet/lipgloss"

// Theme defines all colors used in the TUI.
// Colors are organized by semantic role, not by component.
type Theme struct {
	// Core palette
	Title   lipgloss.Color // Panel titles
	Accent  lipgloss.Color // Hotkeys, active borders, spinners, hover
	Border  lipgloss.Color // Panel border default
	Text    lipgloss.Color // Primary text, selected text
	Label   lipgloss.Color // Field labels
	Dim     lipgloss.Color // Secondary text, pending, tree connectors
	Success lipgloss.Color // Completed, features, approvals
	Error   lipgloss.Color // Failed, bugs, CI failure
	Warning lipgloss.Color // Unassigned badges, changes requested

	// Status colors
	StatusPending    lipgloss.Color
	StatusProcessing lipgloss.Color
	StatusCompleted  lipgloss.Color
	StatusFailed     lipgloss.Color
	StatusIdle       lipgloss.Color

	// Bead type colors
	TypeTask    lipgloss.Color
	TypeBug     lipgloss.Color
	TypeFeature lipgloss.Color
	TypeEpic    lipgloss.Color
	TypeDefault lipgloss.Color

	// Surface colors (backgrounds)
	SurfaceBar      lipgloss.Color // Status bar background
	SurfaceDialog   lipgloss.Color // Dialog/help background
	SurfaceTabBar   lipgloss.Color // Tab bar background
	SurfaceSelected lipgloss.Color // Selected item background

	// Tab bar
	Ribbon        lipgloss.Color // Ribbon/branding background
	RibbonText    lipgloss.Color // Ribbon text
	TabInactiveBg lipgloss.Color
	TabInactiveFg lipgloss.Color
	TabActiveBg   lipgloss.Color
	TabActiveFg   lipgloss.Color

	// Dialog
	DialogBorder lipgloss.Color

	// Highlights & animation
	NewBead           lipgloss.Color // New bead highlight
	NewBeadSelectedBg lipgloss.Color
	NewBeadHoverBg    lipgloss.Color
	HoverBg           lipgloss.Color // General hover background

	// Special indicators
	Cyan      lipgloss.Color // PR URLs, work names, unseen badges
	Merged    lipgloss.Color // Merged state
	CIPending lipgloss.Color
	CISuccess lipgloss.Color

	// Health indicators
	HealthGood lipgloss.Color
	HealthBad  lipgloss.Color

	// Progress colors
	ProgressFull   lipgloss.Color
	ProgressHigh   lipgloss.Color
	ProgressMedium lipgloss.Color
	ProgressLow    lipgloss.Color

	// Splash gradient (top to bottom)
	SplashGradient []lipgloss.Color

	// Chat bubble colors
	ChatUserBg     lipgloss.Color // Solid fill for user messages
	ChatUserFg     lipgloss.Color // Text on user message background
	ChatSystemBrd  lipgloss.Color // Border for system messages
	ChatSystemFg   lipgloss.Color // Text for system messages

	// Utility
	Black lipgloss.Color // Black text for inverted backgrounds
}

// defaultTheme matches the current hardcoded colors exactly.
var defaultTheme = Theme{
	Title:   lipgloss.Color("205"),
	Accent:  lipgloss.Color("214"),
	Border:  lipgloss.Color("62"),
	Text:    lipgloss.Color("255"),
	Label:   lipgloss.Color("247"),
	Dim:     lipgloss.Color("241"),
	Success: lipgloss.Color("42"),
	Error:   lipgloss.Color("196"),
	Warning: lipgloss.Color("214"),

	StatusPending:    lipgloss.Color("241"),
	StatusProcessing: lipgloss.Color("214"),
	StatusCompleted:  lipgloss.Color("42"),
	StatusFailed:     lipgloss.Color("196"),
	StatusIdle:       lipgloss.Color("247"),

	TypeTask:    lipgloss.Color("75"),
	TypeBug:     lipgloss.Color("196"),
	TypeFeature: lipgloss.Color("42"),
	TypeEpic:    lipgloss.Color("213"),
	TypeDefault: lipgloss.Color("247"),

	SurfaceBar:      lipgloss.Color("236"),
	SurfaceDialog:   lipgloss.Color("235"),
	SurfaceTabBar:   lipgloss.Color("235"),
	SurfaceSelected: lipgloss.Color("62"),

	Ribbon:        lipgloss.Color("29"),
	RibbonText:    lipgloss.Color("15"),
	TabInactiveBg: lipgloss.Color("240"),
	TabInactiveFg: lipgloss.Color("255"),
	TabActiveBg:   lipgloss.Color("214"),
	TabActiveFg:   lipgloss.Color("232"),

	DialogBorder: lipgloss.Color("99"),

	NewBead:           lipgloss.Color("#FFFF00"),
	NewBeadSelectedBg: lipgloss.Color("226"),
	NewBeadHoverBg:    lipgloss.Color("228"),
	HoverBg:           lipgloss.Color("240"),

	Cyan:      lipgloss.Color("81"),
	Merged:    lipgloss.Color("141"),
	CIPending: lipgloss.Color("226"),
	CISuccess: lipgloss.Color("82"),

	HealthGood: lipgloss.Color("2"),
	HealthBad:  lipgloss.Color("1"),

	ProgressFull:   lipgloss.Color("82"),
	ProgressHigh:   lipgloss.Color("226"),
	ProgressMedium: lipgloss.Color("214"),
	ProgressLow:    lipgloss.Color("247"),

	ChatUserBg:    lipgloss.Color("62"),  // Purple-ish solid fill
	ChatUserFg:    lipgloss.Color("255"), // White text on user bg
	ChatSystemBrd: lipgloss.Color("241"), // Subtle border for system
	ChatSystemFg:  lipgloss.Color("252"), // Slightly bright text for system

	SplashGradient: []lipgloss.Color{
		lipgloss.Color("#FF6B6B"),
		lipgloss.Color("#FF8E53"),
		lipgloss.Color("#FFA07A"),
		lipgloss.Color("#FFB347"),
		lipgloss.Color("#FFC93C"),
		lipgloss.Color("#FFD700"),
		lipgloss.Color("#FFDF00"),
		lipgloss.Color("#FFE44D"),
		lipgloss.Color("#FFEC80"),
	},

	Black: lipgloss.Color("0"),
}

var currentTheme = defaultTheme

// CurrentTheme returns the active theme.
func CurrentTheme() *Theme {
	return &currentTheme
}

// SetTheme replaces the active theme.
func SetTheme(t Theme) {
	currentTheme = t
}
