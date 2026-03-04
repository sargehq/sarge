package bridge

import "encoding/json"

// EventType identifies the kind of event received from a pi RPC session.
type EventType string

const (
	// Core lifecycle events.
	EventAgentStart  EventType = "agent_start"
	EventAgentEnd    EventType = "agent_end"
	EventTurnStart   EventType = "turn_start"
	EventTurnEnd     EventType = "turn_end"
	EventMessageStart EventType = "message_start"
	EventMessageEnd  EventType = "message_end"

	// Streaming updates.
	EventMessageUpdate EventType = "message_update"

	// Tool execution events.
	EventToolExecutionStart  EventType = "tool_execution_start"
	EventToolExecutionUpdate EventType = "tool_execution_update"
	EventToolExecutionEnd    EventType = "tool_execution_end"

	// Compaction events.
	EventAutoCompactionStart EventType = "auto_compaction_start"
	EventAutoCompactionEnd   EventType = "auto_compaction_end"

	// Retry events.
	EventAutoRetryStart EventType = "auto_retry_start"
	EventAutoRetryEnd   EventType = "auto_retry_end"

	// Extension events.
	EventExtensionError     EventType = "extension_error"
	EventExtensionUIRequest EventType = "extension_ui_request"

	// Response is a command response, not a streaming event.
	EventResponse EventType = "response"
)

// Event represents a single event received from a pi RPC session.
// The Raw field contains the full JSON for callers that need to inspect
// event-specific fields beyond the common ones.
type Event struct {
	// Type identifies the event kind.
	Type EventType `json:"type"`

	// Raw contains the full JSON payload of the event.
	Raw json.RawMessage `json:"-"`
}

// ResponseEvent contains fields specific to command responses.
type ResponseEvent struct {
	Type    EventType       `json:"type"`
	Command string          `json:"command"`
	Success bool            `json:"success"`
	Error   string          `json:"error,omitempty"`
	ID      string          `json:"id,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// MessageUpdateEvent contains fields for streaming message updates.
type MessageUpdateEvent struct {
	Type                  EventType       `json:"type"`
	Message               json.RawMessage `json:"message,omitempty"`
	AssistantMessageEvent *AssistantDelta `json:"assistantMessageEvent,omitempty"`
}

// AssistantDelta describes a streaming delta within a message_update event.
type AssistantDelta struct {
	Type         string          `json:"type"`
	ContentIndex int             `json:"contentIndex,omitempty"`
	Delta        string          `json:"delta,omitempty"`
	Content      string          `json:"content,omitempty"`
	Partial      json.RawMessage `json:"partial,omitempty"`
	ToolCall     json.RawMessage `json:"toolCall,omitempty"`
	Reason       string          `json:"reason,omitempty"`
}

// ToolExecutionStartEvent contains fields for tool_execution_start events.
type ToolExecutionStartEvent struct {
	Type       EventType       `json:"type"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Args       json.RawMessage `json:"args,omitempty"`
}

// ToolExecutionEndEvent contains fields for tool_execution_end events.
type ToolExecutionEndEvent struct {
	Type       EventType       `json:"type"`
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	Result     json.RawMessage `json:"result,omitempty"`
	IsError    bool            `json:"isError"`
}

// AgentEndEvent contains the messages generated during an agent run.
type AgentEndEvent struct {
	Type     EventType       `json:"type"`
	Messages json.RawMessage `json:"messages,omitempty"`
}

// ParseResponseEvent parses the raw JSON of an event into a ResponseEvent.
func ParseResponseEvent(raw json.RawMessage) (*ResponseEvent, error) {
	var r ResponseEvent
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// ParseMessageUpdateEvent parses the raw JSON into a MessageUpdateEvent.
func ParseMessageUpdateEvent(raw json.RawMessage) (*MessageUpdateEvent, error) {
	var m MessageUpdateEvent
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
