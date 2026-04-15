package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"go-hermes-agent/internal/config"
)

type Client struct {
	cfg    config.LLMConfig
	client *http.Client
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Message = message

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []message `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message message `json:"message"`
	} `json:"choices"`
}

func New(cfg config.Config) *Client {
	llmCfg := cfg.ResolvedLLM()
	return &Client{
		cfg: llmCfg,
		client: &http.Client{
			Timeout: time.Duration(llmCfg.TimeoutSeconds) * time.Second,
		},
	}
}

func (c *Client) Chat(ctx context.Context, prompt string) (string, error) {
	return c.ChatWithContext(ctx, nil, prompt)
}

func (c *Client) ChatWithContext(ctx context.Context, systemBlocks []string, prompt string) (string, error) {
	return c.ChatWithMessages(ctx, systemBlocks, nil, prompt)
}

func (c *Client) ChatWithMessages(ctx context.Context, systemBlocks []string, history []message, prompt string) (string, error) {
	apiKey := ""
	if strings.TrimSpace(c.cfg.APIKeyEnv) != "" {
		apiKey = strings.TrimSpace(os.Getenv(c.cfg.APIKeyEnv))
		if apiKey == "" {
			return "", fmt.Errorf("missing API key env %q", c.cfg.APIKeyEnv)
		}
	}
	messages := []message{{Role: "system", Content: "You are a secure, concise assistant."}}
	for _, block := range systemBlocks {
		block = strings.TrimSpace(block)
		if block != "" {
			messages = append(messages, message{Role: "system", Content: block})
		}
	}
	for _, item := range history {
		if strings.TrimSpace(item.Role) == "" || strings.TrimSpace(item.Content) == "" {
			continue
		}
		messages = append(messages, item)
	}
	messages = append(messages, message{Role: "user", Content: prompt})
	reqBody := chatRequest{Model: c.cfg.Model, Messages: messages}
	raw, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.cfg.BaseURL, "/")+"/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("llm request failed with status %s", resp.Status)
	}
	var out chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("empty llm response")
	}
	return out.Choices[0].Message.Content, nil
}

func (c *Client) Config() config.LLMConfig {
	return c.cfg
}

func NewMessage(role, content string) message {
	return message{Role: role, Content: content}
}
