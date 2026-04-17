package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-hermes-agent/internal/config"
)

// Client is a minimal OpenAI-compatible chat completions client.
//
// It supports plain chat requests and native tool-calling for providers that
// expose the standard /chat/completions API shape.
type Client struct {
	cfg    config.LLMConfig
	client *http.Client
}

// ToolFunctionDefinition describes one callable function in OpenAI-compatible format.
type ToolFunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

// ToolDefinition wraps one function tool for the chat completions API.
type ToolDefinition struct {
	Type     string                 `json:"type"`
	Function ToolFunctionDefinition `json:"function"`
}

// ToolFunctionCall is the function payload returned by a native tool call.
type ToolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall is one native tool invocation requested by the model.
type ToolCall struct {
	ID       string           `json:"id,omitempty"`
	Type     string           `json:"type,omitempty"`
	Function ToolFunctionCall `json:"function"`
}

type message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// Message is the chat message shape used by the client.
type Message = message

type chatRequest struct {
	Model      string           `json:"model"`
	Messages   []message        `json:"messages"`
	Tools      []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string           `json:"tool_choice,omitempty"`
}

type rawContent string

func (c *rawContent) UnmarshalJSON(data []byte) error {
	*c = ""
	if string(data) == "null" {
		return nil
	}
	var asString string
	if err := json.Unmarshal(data, &asString); err == nil {
		*c = rawContent(asString)
		return nil
	}
	var parts []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &parts); err == nil {
		var builder strings.Builder
		for _, part := range parts {
			builder.WriteString(part.Text)
		}
		*c = rawContent(builder.String())
		return nil
	}
	return fmt.Errorf("unsupported message content format")
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Role      string     `json:"role"`
			Content   rawContent `json:"content"`
			ToolCalls []ToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ChatCompletion is the normalized response returned by ChatCompletion.
type ChatCompletion struct {
	Message      Message
	FinishReason string
}

// New builds a client from the active application config.
func New(cfg config.Config) *Client {
	return NewFromLLMConfig(cfg.ResolvedLLM())
}

// NewFromLLMConfig builds a client from one explicit resolved LLM config.
func NewFromLLMConfig(llmCfg config.LLMConfig) *Client {
	return &Client{
		cfg: llmCfg,
		client: &http.Client{
			Timeout: time.Duration(llmCfg.TimeoutSeconds) * time.Second,
		},
	}
}

// Chat sends a single user prompt with no extra context blocks.
func (c *Client) Chat(ctx context.Context, prompt string) (string, error) {
	return c.ChatWithContext(ctx, nil, prompt)
}

// ChatWithContext sends a single prompt with additional system blocks.
func (c *Client) ChatWithContext(ctx context.Context, systemBlocks []string, prompt string) (string, error) {
	return c.ChatWithMessages(ctx, systemBlocks, nil, prompt)
}

// ChatWithMessages sends a prompt with system blocks and prior message history.
func (c *Client) ChatWithMessages(ctx context.Context, systemBlocks []string, history []message, prompt string) (string, error) {
	completion, err := c.ChatCompletion(ctx, systemBlocks, history, prompt, nil)
	if err != nil {
		return "", err
	}
	return completion.Message.Content, nil
}

// ChatCompletion sends a normalized chat completion request and optionally
// includes native tool definitions for OpenAI-compatible tool-calling.
func (c *Client) ChatCompletion(ctx context.Context, systemBlocks []string, history []message, prompt string, toolDefs []ToolDefinition) (ChatCompletion, error) {
	apiKey := strings.TrimSpace(c.cfg.APIKey)
	messages := []message{{Role: "system", Content: "You are a secure, concise assistant."}}
	for _, block := range systemBlocks {
		block = strings.TrimSpace(block)
		if block != "" {
			messages = append(messages, message{Role: "system", Content: block})
		}
	}
	for _, item := range history {
		if strings.TrimSpace(item.Role) == "" {
			continue
		}
		if strings.TrimSpace(item.Content) == "" && len(item.ToolCalls) == 0 && item.Role != "tool" {
			continue
		}
		messages = append(messages, item)
	}
	if strings.TrimSpace(prompt) != "" {
		messages = append(messages, message{Role: "user", Content: prompt})
	}
	reqBody := chatRequest{Model: c.cfg.Model, Messages: messages}
	if len(toolDefs) > 0 {
		reqBody.Tools = toolDefs
		reqBody.ToolChoice = "auto"
	}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return ChatCompletion{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ChatCompletion{}, err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return ChatCompletion{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return ChatCompletion{}, fmt.Errorf("llm request failed with status %s", resp.Status)
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ChatCompletion{}, err
	}
	if len(out.Choices) == 0 {
		return ChatCompletion{}, fmt.Errorf("empty llm response")
	}
	choice := out.Choices[0]
	return ChatCompletion{
		Message: Message{
			Role:      choice.Message.Role,
			Content:   strings.TrimSpace(string(choice.Message.Content)),
			ToolCalls: choice.Message.ToolCalls,
		},
		FinishReason: choice.FinishReason,
	}, nil
}

// Config returns the resolved LLM config used by the client.
func (c *Client) Config() config.LLMConfig {
	return c.cfg
}

// NewMessage builds a basic role/content message pair.
func NewMessage(role, content string) message {
	return message{Role: role, Content: content}
}
