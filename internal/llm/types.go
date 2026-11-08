package llm

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type Config struct {
	Model       string
	MaxTokens   int64
	System      string
	CacheSystem bool
	Effort      anthropic.OutputConfigEffort
}

type ToolCall struct {
	ID     string
	Name   string
	Input  json.RawMessage
	Output string // set by the agent after it runs the call; Send leaves it empty
}

type Usage struct {
	InputTokens      int64
	OutputTokens     int64
	CacheWriteTokens int64
	CacheReadTokens  int64
}

// Add sums another call's token counts into this one.
func (usage *Usage) Add(other Usage) {
	usage.InputTokens += other.InputTokens
	usage.OutputTokens += other.OutputTokens
	usage.CacheWriteTokens += other.CacheWriteTokens
	usage.CacheReadTokens += other.CacheReadTokens
}

type Reply struct {
	Message    *anthropic.Message
	Text       string
	Calls      []ToolCall
	Usage      Usage
	StopReason anthropic.StopReason
}
