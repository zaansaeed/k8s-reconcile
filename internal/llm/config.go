package llm

const (
	ModelOpus   = "claude-opus-5-5"
	ModelSonnet = "claude-sonnet-5-5"
	ModelHaiku  = "claude-haiku-5-5"
)

var modelAliases = map[string]string{
	"opus":   ModelOpus,
	"sonnet": ModelSonnet,
	"haiku":  ModelHaiku,
}

func resolveModel(name string) string {
	if id, ok := modelAliases[name]; ok {
		return id
	}
	return name
}

// DefaultConfig returns the settings for the given system prompt and model
// (an alias such as "haiku", or a full ID).
func DefaultConfig(system, model string) Config {
	return Config{
		Model:     resolveModel(model),
		MaxTokens: 8192,
		System:    system,
	}
}
