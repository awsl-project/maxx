package kiro

import (
	"strings"
	"sync"
)

// ToolParamsUpdateCallback notifies when a tool's parameters are fully aggregated.
type ToolParamsUpdateCallback func(toolUseID string, fullParams string)

// StreamingJSONAggregator aggregates tool input fragments across events.
type StreamingJSONAggregator struct {
	activeStreamers map[string]*toolJSONBuffer
	mu              sync.Mutex
	updateCallback  ToolParamsUpdateCallback
}

type toolJSONBuffer struct {
	toolUseID string
	toolName  string
	builder   strings.Builder
}

// NewStreamingJSONAggregatorWithCallback creates an aggregator with callback.
func NewStreamingJSONAggregatorWithCallback(callback ToolParamsUpdateCallback) *StreamingJSONAggregator {
	return &StreamingJSONAggregator{
		activeStreamers: make(map[string]*toolJSONBuffer),
		updateCallback:  callback,
	}
}

// ProcessToolData appends fragments and returns full input on stop.
func (a *StreamingJSONAggregator) ProcessToolData(toolUseID, name, input string, stop bool, _ int) (bool, string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	streamer, exists := a.activeStreamers[toolUseID]
	if !exists {
		streamer = &toolJSONBuffer{toolUseID: toolUseID, toolName: name}
		a.activeStreamers[toolUseID] = streamer
	}

	if input != "" {
		streamer.builder.WriteString(input)
	}

	if !stop {
		return false, ""
	}

	fullInput := strings.TrimSpace(streamer.builder.String())
	if fullInput == "" {
		fullInput = "{}"
	} else {
		var test map[string]any
		if err := FastUnmarshal([]byte(fullInput), &test); err != nil {
			fullInput = "{}"
		}
	}

	delete(a.activeStreamers, toolUseID)

	if a.updateCallback != nil {
		a.updateCallback(toolUseID, fullInput)
	}

	return true, fullInput
}
