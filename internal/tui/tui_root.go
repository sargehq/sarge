package tui

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	"github.com/sargehq/sarge/internal/control"
	"github.com/sargehq/sarge/internal/db"
	"github.com/sargehq/sarge/internal/logging"
	"github.com/sargehq/sarge/internal/procmon"
	"github.com/sargehq/sarge/internal/project"
	"github.com/sargehq/sarge/internal/taskseq"
	trackingwatcher "github.com/sargehq/sarge/internal/tracking/watcher"
)

// Bubblezone setup: NewGlobal() initializes the global zone manager.
// Child components use zone.NewPrefix() for unique IDs; zone.Scan() is called only here at the root.
func init() {
	zone.NewGlobal()
}

// rootModel is the top-level TUI model
type rootModel struct {
	ctx    context.Context
	proj   *project.Project
	width  int
	height int

	// The plan model (our only model now)
	planModel *planModel

	// Global state
	spinner    spinner.Model
	lastUpdate time.Time
	quitting   bool

	// Mouse state
	mouseX int
	mouseY int
}

// newRootModel creates a new root TUI model
func newRootModel(ctx context.Context, proj *project.Project) rootModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))

	// Create the plan model
	planModel := newPlanModel(ctx, proj)

	return rootModel{
		ctx:        ctx,
		proj:       proj,
		width:      80,
		height:     24,
		planModel:  planModel,
		spinner:    s,
		lastUpdate: time.Now(),
	}
}

// Init implements tea.Model
func (m rootModel) Init() tea.Cmd {
	// Initialize plan model
	if m.planModel != nil {
		return m.planModel.Init()
	}
	return nil
}

// Update implements tea.Model
func (m rootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Update plan model dimensions
		if m.planModel != nil {
			m.planModel.SetSize(m.width, m.height)
		}

		return m, nil

	case tea.MouseMsg:
		m.mouseX = msg.X
		m.mouseY = msg.Y

		// Route mouse events directly to plan model
		if m.planModel != nil {
			var cmd tea.Cmd
			var newModel tea.Model
			newModel, cmd = m.planModel.Update(msg)
			m.planModel = newModel.(*planModel)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		// Check if plan model is in modal state - if so, route directly to it
		if m.planModel != nil && m.planModel.InModal() {
			var cmd tea.Cmd
			var newModel tea.Model
			newModel, cmd = m.planModel.Update(msg)
			m.planModel = newModel.(*planModel)
			return m, cmd
		}

		// Global keys (only when not in modal)
		switch msg.String() {
		case "q":
			m.quitting = true
			// Clean up resources in plan model
			if m.planModel != nil {
				m.planModel.cleanup()
			}
			return m, tea.Quit
		}

		// Route to plan model
		if m.planModel != nil {
			var cmd tea.Cmd
			var newModel tea.Model
			newModel, cmd = m.planModel.Update(msg)
			m.planModel = newModel.(*planModel)
			return m, cmd
		}
		return m, nil

	default:
		// Route other messages to plan model
		if m.planModel != nil {
			var cmd tea.Cmd
			var newModel tea.Model
			newModel, cmd = m.planModel.Update(msg)
			m.planModel = newModel.(*planModel)
			return m, cmd
		}
		return m, nil
	}
}

// View implements tea.Model
func (m rootModel) View() string {
	if m.quitting {
		return ""
	}

	// Render plan model content directly and wrap with zone.Scan
	if m.planModel != nil {
		return zone.Scan(m.planModel.View())
	}

	return ""
}

// RunRootTUI starts the TUI with the new root model.
// It also starts the control plane and task sequencer as in-process goroutines.
func RunRootTUI(ctx context.Context, proj *project.Project, enableMouse bool) error {
	// Apply hooks.env so child processes (bridge sessions) inherit them.
	for _, e := range proj.Config.Hooks.Env {
		idx := -1
		for i, c := range e {
			if c == '=' {
				idx = i
				break
			}
		}
		if idx > 0 {
			_ = os.Setenv(e[:idx], os.ExpandEnv(e[idx+1:])) //nolint:errcheck // best-effort env setup
		}
	}
	_ = os.Setenv("BEANS_PATH", proj.BeansPath())

	// Create a cancellable context for background goroutines.
	bgCtx, bgCancel := context.WithCancel(ctx)
	defer bgCancel()

	// Start the control plane as an in-process goroutine.
	procManager := procmon.NewManager(proj.DB, db.DefaultHeartbeatInterval)
	if err := procManager.RegisterControlPlane(bgCtx); err != nil {
		logging.Warn("failed to register control plane process", "error", err)
	}
	defer procManager.Stop()

	go func() {
		err := control.RunControlPlaneLoop(bgCtx, proj, procManager)
		if err != nil && err != control.ErrBinaryChanged {
			logging.Warn("in-process control plane exited", "error", err)
		}
	}()

	// Start the task sequencer as an in-process goroutine.
	model := newRootModel(ctx, proj)
	sequencer := taskseq.New(proj, model.planModel.bridgeClient)

	// Wire up DB watcher to notify the sequencer on changes.
	trackingDBPath := filepath.Join(proj.Root, ".co", "tracking.db")
	seqWatcher, err := trackingwatcher.New(trackingwatcher.DefaultConfig(trackingDBPath))
	if err != nil {
		logging.Warn("failed to create sequencer DB watcher", "error", err)
	} else {
		if err := seqWatcher.Start(); err != nil {
			logging.Warn("failed to start sequencer DB watcher", "error", err)
			_ = seqWatcher.Stop()
			seqWatcher = nil
		} else {
			defer func() {
				if err := seqWatcher.Stop(); err != nil {
					logging.Warn("failed to stop sequencer DB watcher", "error", err)
				}
			}()
			// Forward DB change events to sequencer.
			seqSub := seqWatcher.Broker().Subscribe(bgCtx)
			go func() {
				for {
					select {
					case <-bgCtx.Done():
						return
					case evt, ok := <-seqSub:
						if !ok {
							return
						}
						if evt.Payload.Type == trackingwatcher.DBChanged {
							sequencer.Notify()
						}
					}
				}
			}()
		}
	}

	go func() {
		if err := sequencer.Run(bgCtx); err != nil {
			logging.Warn("task sequencer exited with error", "error", err)
		}
	}()

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if enableMouse {
		opts = append(opts, tea.WithMouseAllMotion())
	}
	p := tea.NewProgram(model, opts...)

	if _, err := p.Run(); err != nil {
		return err
	}

	// Cancel background goroutines and wait for sequencer to finish.
	bgCancel()
	sequencer.Wait()

	return nil
}
