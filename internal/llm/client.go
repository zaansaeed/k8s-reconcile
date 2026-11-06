package llm

import (
	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

const (
	ModelOpus   = "claude-opus-5-5"
	ModelSonnet = "claude-sonnet-5-5"
	ModelHaiku  = "claude-haiku-5-5"
)

type Config struct {
	Model       string
	MaxTokens   int64
	System      string
	CacheSystem bool
	Effort      anthropic.OutputConfigEffort
}

func DefaultConfig(system string) Config {
	return Config{
		Model:       ModelOpus,
		MaxTokens:   8192,
		System:      system,
		CacheSystem: true,
		Effort:      anthropic.OutputConfigEffortMedium,
	}
}

type Client struct {
	api  anthropic.Client //client sdk
	cfg  Config  //config
	defs []anthropic.ToolUnionParam //tools
}

func New(cfg Config, specs []tools.Spec) *Client {
	return &Client{
		api:  anthropic.NewClient(),
		cfg:  cfg,
		defs: toolParams(specs),
	}
}

func toolParams(specs []tools.Spec) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(specs)) //initialize empty array max len 3
	for _, s := range specs { //loop over specs (our tools)
		required := s.Required
		if required == nil {
			required = []string{}
		}
		schema := anthropic.ToolInputSchemaParam{
			Properties: s.Properties,
			Required:   required,
		}
		schema.ExtraFields = map[string]any{"additionalProperties": false}

		t := anthropic.ToolParam{ //convert our spec into an anthropic.ToolParam object
			Name:        s.Name,
			Description: anthropic.String(s.Description),
			InputSchema: schema,
			Strict:      anthropic.Bool(true),
		}
		out = append(out, anthropic.ToolUnionParam{OfTool: &t})
	}
	return out
}
