package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sargehq/sarge/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMockPi creates a shell script that acts as a mock pi RPC process.
// It reads JSON commands from stdin and writes JSON events to stdout.
func writeMockPi(t *testing.T, dir string, script string) string {
	t.Helper()
	path := filepath.Join(dir, "pi")
	err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755)
	require.NoError(t, err)
	return path
}

// TestSessionStates verifies the session state transitions.
func TestSessionStates(t *testing.T) {
	assert.Equal(t, "starting", SessionStarting.String())
	assert.Equal(t, "ready", SessionReady.String())
	assert.Equal(t, "streaming", SessionStreaming.String())
	assert.Equal(t, "dead", SessionDead.String())
}

// TestBridgeSpawnAndKill verifies basic spawn and kill lifecycle using a mock pi.
func TestBridgeSpawnAndKill(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Create a mock pi that reads stdin and exits cleanly.
	// It sends an agent_start event, waits for stdin to close, then exits.
	mockScript := `
echo '{"type":"agent_start"}'
# Read commands until stdin closes
while IFS= read -r line; do
	type=$(echo "$line" | grep -o '"type":"[^"]*"' | head -1 | sed 's/"type":"//;s/"//')
	case "$type" in
		prompt)
			echo '{"type":"response","command":"prompt","success":true}'
			echo '{"type":"agent_start"}'
			echo '{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"Hello"}}'
			echo '{"type":"agent_end","messages":[]}'
			;;
		abort)
			echo '{"type":"response","command":"abort","success":true}'
			;;
		get_state)
			echo '{"type":"response","command":"get_state","success":true,"data":{"isStreaming":false}}'
			;;
	esac
done
`
	mockPath := writeMockPi(t, dir, mockScript)

	// Override PATH so "pi" resolves to our mock.
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	b := NewBridge()
	defer b.KillAll()

	// Spawn a session.
	cfg := SessionConfig{WorkDir: dir}
	session, err := b.SpawnSession("test-1", cfg)
	require.NoError(t, err)
	require.NotNil(t, session)

	assert.Equal(t, "test-1", session.ID())
	assert.Equal(t, SessionReady, session.State())

	// Verify it shows up in listing.
	sessions := b.ListSessions()
	assert.Contains(t, sessions, "test-1")

	// Verify duplicate spawn fails.
	_, err = b.SpawnSession("test-1", cfg)
	assert.Error(t, err)

	// Lookup by ID.
	found := b.GetSession("test-1")
	assert.Equal(t, session, found)

	// Lookup non-existent.
	assert.Nil(t, b.GetSession("nonexistent"))

	// Kill.
	err = b.KillSession("test-1")
	assert.NoError(t, err)

	// Wait for death.
	_ = session.Wait()
	assert.Equal(t, SessionDead, session.State())

	// Kill non-existent session.
	err = b.KillSession("nonexistent")
	assert.Error(t, err)

	_ = mockPath // used via PATH
}

// TestSessionPromptAndEvents verifies sending a prompt and receiving events.
func TestSessionPromptAndEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	mockScript := `
# Wait for a prompt command, then respond.
while IFS= read -r line; do
	type=$(echo "$line" | grep -o '"type":"[^"]*"' | head -1 | sed 's/"type":"//;s/"//')
	case "$type" in
		prompt)
			echo '{"type":"response","command":"prompt","success":true}'
			echo '{"type":"agent_start"}'
			echo '{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"Hello world"}}'
			echo '{"type":"agent_end","messages":[]}'
			;;
		abort)
			echo '{"type":"response","command":"abort","success":true}'
			exit 0
			;;
	esac
done
`
	writeMockPi(t, dir, mockScript)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	b := NewBridge()
	defer b.KillAll()

	session, err := b.SpawnSession("prompt-test", SessionConfig{WorkDir: dir})
	require.NoError(t, err)

	// Send a prompt.
	err = session.Prompt("Hello!")
	require.NoError(t, err)

	// Collect events with timeout.
	var events []Event
	timeout := time.After(5 * time.Second)
	done := false
	for !done {
		select {
		case evt, ok := <-session.Events():
			if !ok {
				done = true
				break
			}
			events = append(events, evt)
			if evt.Type == EventAgentEnd {
				done = true
			}
		case <-timeout:
			t.Fatal("timed out waiting for events")
		}
	}

	// Verify we got the expected event types.
	var types []EventType
	for _, e := range events {
		types = append(types, e.Type)
	}
	assert.Contains(t, types, EventResponse)
	assert.Contains(t, types, EventAgentStart)
	assert.Contains(t, types, EventMessageUpdate)
	assert.Contains(t, types, EventAgentEnd)

	// Verify the message_update event content.
	for _, e := range events {
		if e.Type == EventMessageUpdate {
			mu, err := ParseMessageUpdateEvent(e.Raw)
			require.NoError(t, err)
			assert.Equal(t, "text_delta", mu.AssistantMessageEvent.Type)
			assert.Equal(t, "Hello world", mu.AssistantMessageEvent.Delta)
			break
		}
	}

	// Clean up: send abort to terminate mock.
	_ = session.Abort()
}

// TestBridgeKillAll verifies that KillAll terminates all sessions.
func TestBridgeKillAll(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	mockScript := `
while IFS= read -r line; do
	:
done
`
	writeMockPi(t, dir, mockScript)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	b := NewBridge()

	_, err := b.SpawnSession("s1", SessionConfig{WorkDir: dir})
	require.NoError(t, err)
	_, err = b.SpawnSession("s2", SessionConfig{WorkDir: dir})
	require.NoError(t, err)

	sessions := b.ListSessions()
	assert.Len(t, sessions, 2)

	err = b.KillAll()
	assert.NoError(t, err)

	sessions = b.ListSessions()
	assert.Len(t, sessions, 0)
}

// TestSessionDeadRespawn verifies that a dead session can be replaced by a new spawn.
func TestSessionDeadRespawn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()

	// Mock pi that exits immediately.
	mockScript := `exit 0`
	writeMockPi(t, dir, mockScript)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	b := NewBridge()
	defer b.KillAll()

	s1, err := b.SpawnSession("respawn-test", SessionConfig{WorkDir: dir})
	require.NoError(t, err)

	// Wait for it to die.
	_ = s1.Wait()
	assert.Equal(t, SessionDead, s1.State())

	// Should be able to spawn a new session with the same ID.
	s2, err := b.SpawnSession("respawn-test", SessionConfig{WorkDir: dir})
	require.NoError(t, err)
	assert.NotNil(t, s2)
}

// TestEventParsing verifies JSON round-tripping of events.
func TestEventParsing(t *testing.T) {
	// Test ResponseEvent parsing.
	raw := json.RawMessage(`{"type":"response","command":"prompt","success":true,"id":"req-1"}`)
	resp, err := ParseResponseEvent(raw)
	require.NoError(t, err)
	assert.Equal(t, EventResponse, resp.Type)
	assert.Equal(t, "prompt", resp.Command)
	assert.True(t, resp.Success)
	assert.Equal(t, "req-1", resp.ID)

	// Test error response.
	raw = json.RawMessage(`{"type":"response","command":"set_model","success":false,"error":"not found"}`)
	resp, err = ParseResponseEvent(raw)
	require.NoError(t, err)
	assert.False(t, resp.Success)
	assert.Equal(t, "not found", resp.Error)

	// Test MessageUpdateEvent parsing.
	raw = json.RawMessage(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","contentIndex":0,"delta":"hi"}}`)
	mu, err := ParseMessageUpdateEvent(raw)
	require.NoError(t, err)
	assert.Equal(t, "text_delta", mu.AssistantMessageEvent.Type)
	assert.Equal(t, "hi", mu.AssistantMessageEvent.Delta)
}

// TestSessionConfigFromProject verifies config translation.
func TestSessionConfigFromProject(t *testing.T) {
	cfg := &project.Config{}
	cfg.Pi.Provider = "anthropic"
	cfg.Pi.Model = "claude-sonnet-4"
	cfg.Pi.Thinking = "high"

	sc := SessionConfigFromProject("/work", cfg)
	assert.Equal(t, "anthropic", sc.Provider)
	assert.Equal(t, "claude-sonnet-4", sc.Model)
	assert.Equal(t, "high", sc.Thinking)
	assert.Equal(t, "/work", sc.WorkDir)

	// Nil config should not panic.
	sc = SessionConfigFromProject("/work", nil)
	assert.Equal(t, "/work", sc.WorkDir)
	assert.Empty(t, sc.Provider)
}

// TestRemoveDead verifies dead session cleanup.
func TestRemoveDead(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	dir := t.TempDir()
	mockScript := `exit 0`
	writeMockPi(t, dir, mockScript)
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+origPath)

	b := NewBridge()
	defer b.KillAll()

	s, err := b.SpawnSession("dead-test", SessionConfig{WorkDir: dir})
	require.NoError(t, err)
	_ = s.Wait()

	assert.Len(t, b.ListSessions(), 1)
	b.RemoveDead()
	assert.Len(t, b.ListSessions(), 0)
}
