package llm

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go/v2"

	"github.com/zaansaeed/k8s-reconcile/internal/tools"
)

type Client struct {
	api      anthropic.Client           //client sdk
	config   Config                     //config
	toolDefs []anthropic.ToolUnionParam //tools
}

// New returns a Client for config, converting the tool specs once up front.
func New(config Config, specs []tools.Spec) *Client {
	return &Client{
		api:      anthropic.NewClient(),
		config:   config,
		toolDefs: toolParams(specs),
	}
}

// toolParams converts our neutral tool specs into the SDK's tool definitions.
func toolParams(specs []tools.Spec) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, 0, len(specs)) //initialize empty array max len 3
	for _, spec := range specs {                              //loop over specs (our tools)
		required := spec.Required
		if required == nil {
			required = []string{}
		}
		schema := anthropic.ToolInputSchemaParam{
			Properties: spec.Properties,
			Required:   required,
		}
		schema.ExtraFields = map[string]any{"additionalProperties": false}

		tool := anthropic.ToolParam{ //convert our spec into an anthropic.ToolParam object
			Name:        spec.Name,
			Description: anthropic.String(spec.Description),
			InputSchema: schema,
			Strict:      anthropic.Bool(true),
		}
		params = append(params, anthropic.ToolUnionParam{OfTool: &tool})
	}
	return params
}

// Model reports which model this client calls, for cost accounting.
func (client *Client) Model() string {
	return client.config.Model
}

// Send makes one API call with the conversation so far and returns the
// assistant's text, the tools it wants run, and what the call cost in tokens.
func (client *Client) Send(ctx context.Context, messages []anthropic.MessageParam) (*Reply, error) {
	system := []anthropic.TextBlockParam{{Text: client.config.System}} //prompt
	if client.config.CacheSystem {
		system[0].CacheControl = anthropic.NewCacheControlEphemeralParam()
	}

	response, err := client.api.Messages.New(ctx, anthropic.MessageNewParams{ // send to client.
		Model:        anthropic.Model(client.config.Model),
		MaxTokens:    client.config.MaxTokens,
		System:       system,
		Messages:     messages,
		Tools:        client.toolDefs,
		OutputConfig: anthropic.OutputConfigParam{Effort: client.config.Effort},
	})
	if err != nil {
		return nil, fmt.Errorf("messages call to %s: %w", client.config.Model, err)
	}

	reply := &Reply{
		Message:    response,
		StopReason: response.StopReason,
		Usage: Usage{
			InputTokens:      response.Usage.InputTokens,
			OutputTokens:     response.Usage.OutputTokens,
			CacheWriteTokens: response.Usage.CacheCreationInputTokens,
			CacheReadTokens:  response.Usage.CacheReadInputTokens,
		},
	}

	for _, contentBlock := range response.Content { //iterate over each block in the responses content. we will add this to the reply object's text and calls it wants to preform
		switch block := contentBlock.AsAny().(type) {
		case anthropic.TextBlock:
			reply.Text += block.Text
		case anthropic.ToolUseBlock:
			reply.Calls = append(reply.Calls, ToolCall{
				ID:    block.ID,
				Name:  block.Name,
				Input: json.RawMessage(block.JSON.Input.Raw()),
			})
		}
	}
	return reply, nil
}

// ToolResults wraps one turn's tool output, keyed by tool use ID, as the next
// user message.
func ToolResults(results map[string]string) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(results))
	for toolUseID, output := range results {
		blocks = append(blocks, anthropic.NewToolResultBlock(toolUseID, output, false))
	}
	return anthropic.NewUserMessage(blocks...)
}
