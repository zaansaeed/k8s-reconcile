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
		toolDefs: toolParams(specs), //our specs -> SDK's tool definitions
	}
}

// toolParams converts our neutral tool specs into the SDK's tool definitions.
func toolParams(specs []tools.Spec) []anthropic.ToolUnionParam {
	params := make([]anthropic.ToolUnionParam, 0, len(specs))
	for _, spec := range specs {
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

// params assembles one request from the client's standing settings and the
// conversation so far.
func (client *Client) params(messages []anthropic.MessageParam) anthropic.MessageNewParams {
	// The server renders a request as tools -> system -> messages, so a
	// breakpoint on the system block caches every tool schema and the system
	// prompt as one entry. Those bytes are identical in every run, so each run
	// reads them instead of writing them again.
	system := []anthropic.TextBlockParam{{Text: client.config.System, CacheControl: anthropic.NewCacheControlEphemeralParam()}}

	params := anthropic.MessageNewParams{
		Model:     anthropic.Model(client.config.Model),
		MaxTokens: client.config.MaxTokens,
		System:    system,
		Messages:  messages,
		Tools:     client.toolDefs,
	}
	// Automatic caching moves a second breakpoint to the end of each request,
	// so every step reads the conversation so far from the cache. A one-shot
	// request, like the judge's, would only pay to write it, so it is opt-in.
	if client.config.CacheHistory {
		params.CacheControl = anthropic.NewCacheControlEphemeralParam()
	}
	return params
}

// Send makes one API call with the conversation so far and returns the
// assistant's text, the tools it wants run, and what the call cost in tokens.
func (client *Client) Send(ctx context.Context, messages []anthropic.MessageParam) (*Reply, error) {
	response, err := client.api.Messages.New(ctx, client.params(messages)) // send to client.
	if err != nil {
		return nil, fmt.Errorf("messages call to %s: %w", client.config.Model, err)
	}

	reply := &Reply{
		Message: response,
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

// ToolResults wraps the outputs of one turn's calls as the next user message, in
// the order the model made them, so the same calls always produce the same
// request bytes.
func ToolResults(calls []ToolCall) anthropic.MessageParam {
	blocks := make([]anthropic.ContentBlockParamUnion, 0, len(calls))
	for _, c := range calls {
		blocks = append(blocks, anthropic.NewToolResultBlock(c.ID, c.Output, false))
	}
	return anthropic.NewUserMessage(blocks...)
}
