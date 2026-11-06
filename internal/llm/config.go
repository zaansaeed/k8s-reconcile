package llm

import "github.com/anthropics/anthropic-sdk-go/v2"

const (
	ModelOpus   = "claude-opus-5-5"
	ModelSonnet = "claude-sonnet-5-5"
	ModelHaiku  = "claude-haiku-5-5"
)

// DefaultConfig returns the settings the single-agent baseline run uses.
func DefaultConfig(system string) Config {
	return Config{
		Model:       ModelOpus,
		MaxTokens:   8192,
		System:      system,
		CacheSystem: true,
		Effort:      anthropic.OutputConfigEffortMedium,
	}
}
