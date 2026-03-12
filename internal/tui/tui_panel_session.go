package tui

import (
	tea "charm.land/bubbletea/v2"
	"github.com/sargehq/sarge/internal/ptysession"
)

// SessionPanelAction represents actions from the session panel.
type SessionPanelAction int

const (
	SessionPanelActionNone SessionPanelAction = iota
	SessionPanelActionExit                    // User wants to leave the session viewer
)

// SessionPanel renders the output of a PTY-backed pi session using a virtual
// terminal emulator. All keyboard input is forwarded directly to the PTY —
// pi handles its own UI, prompting, abort (ctrl+c), etc.
type SessionPanel struct {
	// Dimensions
	width  int
	height int

	// Focus state
	focused bool

	// Active PTY session
	session     *ptysession.Session
	sessionID   string
	sessionType string // "orch", "agent", "plan"
}

// NewSessionPanel creates a new SessionPanel.
func NewSessionPanel() *SessionPanel {
	return &SessionPanel{
		width:  80,
		height: 20,
	}
}

// SetSize updates the panel dimensions and propagates to the PTY session.
// The full width/height is given to the PTY — no borders or chrome.
// Only resizes the PTY when dimensions actually change to avoid a
// resize → redraw → output → render → resize feedback loop.
func (p *SessionPanel) SetSize(width, height int) {
	if p.width == width && p.height == height {
		return
	}
	p.width = width
	p.height = height

	if p.session != nil {
		p.session.Resize(width, height)
	}
}

// SetFocus updates the focus state.
func (p *SessionPanel) SetFocus(focused bool) {
	p.focused = focused
}

// SetSession attaches a PTY session to this panel.
func (p *SessionPanel) SetSession(session *ptysession.Session, sessionType string) {
	p.session = session
	if session != nil {
		p.sessionID = session.ID()
	} else {
		p.sessionID = ""
	}
	p.sessionType = sessionType

	// Resize the session to match the panel.
	if session != nil {
		session.Resize(p.width, p.height)
	}
}

// Session returns the currently attached PTY session, or nil.
func (p *SessionPanel) Session() *ptysession.Session {
	return p.session
}

// Clear detaches the current session without killing it.
func (p *SessionPanel) Clear() {
	p.session = nil
	p.sessionID = ""
	p.sessionType = ""
}

// Update handles key events. In PTY mode, we forward raw input directly to
// the PTY. The only key we intercept is a double-Esc to exit the viewer.
func (p *SessionPanel) Update(msg tea.KeyPressMsg) (tea.Cmd, SessionPanelAction) {
	if p.session == nil || p.session.State() == ptysession.SessionDead {
		// Session is dead or missing — esc exits.
		if msg.String() == "esc" {
			return nil, SessionPanelActionExit
		}
		return nil, SessionPanelActionNone
	}

	// Forward raw key data to the PTY. Bubbletea gives us the raw bytes
	// via msg.String() for simple keys. For special keys we need to convert.
	raw := keyMsgToBytes(msg)
	if len(raw) > 0 {
		_ = p.session.WriteInput(raw)
	}

	return nil, SessionPanelActionNone
}

// Render returns the session panel content (the virtual terminal output).
func (p *SessionPanel) Render() string {
	if p.session == nil {
		return tuiDimStyle.Render("No active session")
	}
	return p.session.Render()
}

// RenderWithPanel returns the session output directly — no borders or chrome.
// Pi renders its own UI natively via the PTY.
func (p *SessionPanel) RenderWithPanel(contentHeight int) string {
	return p.Render()
}

// keyMsgToBytes converts a bubbletea KeyPressMsg to raw bytes suitable for a PTY.
func keyMsgToBytes(msg tea.KeyPressMsg) []byte {
	// Handle Ctrl+key combinations first
	if msg.Mod&tea.ModCtrl != 0 {
		if msg.Code >= 'a' && msg.Code <= 'z' {
			return []byte{byte(msg.Code - 'a' + 1)}
		}
	}

	// Handle special keys by Code
	switch msg.Code {
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeyBackspace:
		return []byte{127}
	case tea.KeyEscape:
		return []byte{27}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyInsert:
		return []byte("\x1b[2~")
	case tea.KeyF1:
		return []byte("\x1bOP")
	case tea.KeyF2:
		return []byte("\x1bOQ")
	case tea.KeyF3:
		return []byte("\x1bOR")
	case tea.KeyF4:
		return []byte("\x1bOS")
	case tea.KeyF5:
		return []byte("\x1b[15~")
	case tea.KeyF6:
		return []byte("\x1b[17~")
	case tea.KeyF7:
		return []byte("\x1b[18~")
	case tea.KeyF8:
		return []byte("\x1b[19~")
	case tea.KeyF9:
		return []byte("\x1b[20~")
	case tea.KeyF10:
		return []byte("\x1b[21~")
	case tea.KeyF11:
		return []byte("\x1b[23~")
	case tea.KeyF12:
		return []byte("\x1b[24~")
	}

	// For printable characters, use the Text field
	if msg.Text != "" {
		return []byte(msg.Text)
	}

	// Fallback: try string representation
	s := msg.String()
	if s != "" {
		return []byte(s)
	}
	return nil
}
