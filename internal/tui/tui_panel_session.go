package tui

import (
	tea "github.com/charmbracelet/bubbletea"
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
func (p *SessionPanel) Update(msg tea.KeyMsg) (tea.Cmd, SessionPanelAction) {
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

// keyMsgToBytes converts a bubbletea KeyMsg to raw bytes suitable for a PTY.
func keyMsgToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
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
	case tea.KeyCtrlA:
		return []byte{1}
	case tea.KeyCtrlB:
		return []byte{2}
	case tea.KeyCtrlC:
		return []byte{3}
	case tea.KeyCtrlD:
		return []byte{4}
	case tea.KeyCtrlE:
		return []byte{5}
	case tea.KeyCtrlF:
		return []byte{6}
	case tea.KeyCtrlG:
		return []byte{7}
	case tea.KeyCtrlH:
		return []byte{8}
	case tea.KeyCtrlK:
		return []byte{11}
	case tea.KeyCtrlL:
		return []byte{12}
	case tea.KeyCtrlN:
		return []byte{14}
	case tea.KeyCtrlO:
		return []byte{15}
	case tea.KeyCtrlP:
		return []byte{16}
	case tea.KeyCtrlR:
		return []byte{18}
	case tea.KeyCtrlS:
		return []byte{19}
	case tea.KeyCtrlT:
		return []byte{20}
	case tea.KeyCtrlU:
		return []byte{21}
	case tea.KeyCtrlV:
		return []byte{22}
	case tea.KeyCtrlW:
		return []byte{23}
	case tea.KeyCtrlX:
		return []byte{24}
	case tea.KeyCtrlY:
		return []byte{25}
	case tea.KeyCtrlZ:
		return []byte{26}
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
	default:
		// For anything else, try the string representation.
		s := msg.String()
		if s != "" {
			return []byte(s)
		}
		return nil
	}
}
