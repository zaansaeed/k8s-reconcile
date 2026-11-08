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

func ResolveModel(name string) string {
	if id, ok := modelAliases[name]; ok {
		return id
	}
	return name
}

// DefaultConfig returns the settings the single-agent baseline run uses.
func DefaultConfig(system string) Config {
	return Config{
		Model:       ModelOpus,
		MaxTokens:   8192,
		System:      system,
		CacheSystem: true,
	}
}
