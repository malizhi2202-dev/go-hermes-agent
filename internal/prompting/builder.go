package prompting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"go-hermes-agent/internal/contextengine"
	"go-hermes-agent/internal/llm"
	"go-hermes-agent/internal/store"
)

// BuildInput describes the sources needed to assemble one prompt.
type BuildInput struct {
	Username             string
	Prompt               string
	Model                string
	HistoryWindow        int
	MaxPromptChars       int
	SummaryStrategy      string
	CompressionEnabled   bool
	CompressionSummaries bool
}

// BuildResult is the fully assembled prompt plan used by chat and inspection flows.
type BuildResult struct {
	Model                 string               `json:"model"`
	Prompt                string               `json:"prompt"`
	SystemBlocks          []string             `json:"system_blocks"`
	History               []llm.Message        `json:"history"`
	MemoryContext         string               `json:"memory_context,omitempty"`
	PersistedSummary      string               `json:"persisted_summary,omitempty"`
	Compression           contextengine.Result `json:"compression"`
	PromptChars           int                  `json:"prompt_chars"`
	MaxPromptChars        int                  `json:"max_prompt_chars"`
	HistoryWindowMessages int                  `json:"history_window_messages"`
	HistoryMessagesUsed   int                  `json:"history_messages_used"`
	SystemBlocksUsed      int                  `json:"system_blocks_used"`
	SummaryUpdated        bool                 `json:"summary_updated"`
	CacheHit              bool                 `json:"cache_hit"`
	CacheKey              string               `json:"cache_key,omitempty"`
	Metadata              map[string]any       `json:"metadata,omitempty"`
}

// Dependencies keeps the builder independent from concrete store or memory implementations.
type Dependencies struct {
	PrefetchMemory func(ctx context.Context, username, prompt string) (string, error)
	GetSummary     func(ctx context.Context, username string) (store.ContextSummary, error)
	PersistSummary func(ctx context.Context, username, summary, strategy string) error
	ListRecent     func(ctx context.Context, username string, limit int) ([]store.Message, error)
	Compress       func(ctx context.Context, existingSummary string, history []llm.Message) contextengine.Result
}

// Builder assembles prompt plans and optionally caches them.
type Builder struct {
	deps  Dependencies
	cache *Cache
}

// NewBuilder creates a prompt builder with explicit dependencies.
func NewBuilder(deps Dependencies, cache *Cache) *Builder {
	return &Builder{deps: deps, cache: cache}
}

// Build assembles the prompt context used for one chat or inspection turn.
func (b *Builder) Build(ctx context.Context, input BuildInput) (BuildResult, error) {
	memoryContext, err := b.deps.PrefetchMemory(ctx, input.Username, input.Prompt)
	if err != nil {
		return BuildResult{}, err
	}
	storedSummary, err := b.deps.GetSummary(ctx, input.Username)
	if err != nil {
		return BuildResult{}, err
	}
	recentMessages, err := b.deps.ListRecent(ctx, input.Username, input.HistoryWindow)
	if err != nil {
		return BuildResult{}, err
	}
	history := recentStoreMessagesToHistory(recentMessages)
	compression := b.deps.Compress(ctx, storedSummary.Summary, history)
	systemBlocks := make([]string, 0, 2)
	if memoryContext != "" {
		systemBlocks = append(systemBlocks, memoryContext)
	}
	summaryText := storedSummary.Summary
	if summaryText != "" {
		systemBlocks = append(systemBlocks, summaryText)
	}
	summaryUpdated := false
	if compression.Compressed && compression.SystemBlock != "" {
		if summaryText == "" {
			systemBlocks = append(systemBlocks, compression.SystemBlock)
		} else {
			systemBlocks[len(systemBlocks)-1] = compression.SystemBlock
		}
		history = compression.History
		if strings.TrimSpace(compression.PersistedSummary) != "" && compression.PersistedSummary != storedSummary.Summary {
			if err := b.deps.PersistSummary(ctx, input.Username, compression.PersistedSummary, input.SummaryStrategy); err != nil {
				return BuildResult{}, err
			}
			summaryUpdated = true
			summaryText = compression.PersistedSummary
		}
	}
	cacheKey := buildCacheKey(input, memoryContext, summaryText, history)
	if b.cache != nil {
		if cached, ok := b.cache.Get(cacheKey); ok {
			return cached, nil
		}
	}
	trimmed := trimHistoryToBudget(systemBlocks, history, input.Prompt, input.MaxPromptChars)
	promptChars := len(input.Prompt) + len("You are a secure, concise assistant.")
	for _, block := range systemBlocks {
		promptChars += len(block)
	}
	for _, item := range trimmed {
		promptChars += len(item.Content)
	}
	result := BuildResult{
		Model:                 input.Model,
		Prompt:                input.Prompt,
		SystemBlocks:          systemBlocks,
		History:               trimmed,
		MemoryContext:         memoryContext,
		PersistedSummary:      summaryText,
		Compression:           compression,
		PromptChars:           promptChars,
		MaxPromptChars:        input.MaxPromptChars,
		HistoryWindowMessages: input.HistoryWindow,
		HistoryMessagesUsed:   len(trimmed),
		SystemBlocksUsed:      len(systemBlocks),
		SummaryUpdated:        summaryUpdated,
		CacheKey:              cacheKey,
		Metadata: map[string]any{
			"summary_strategy":      input.SummaryStrategy,
			"compression_enabled":   input.CompressionEnabled,
			"persisted_summary_len": len(summaryText),
		},
	}
	if b.cache != nil {
		b.cache.Set(cacheKey, result)
	}
	return result, nil
}

// CacheStats returns prompt cache counters when caching is enabled.
func (b *Builder) CacheStats() CacheStats {
	if b == nil || b.cache == nil {
		return CacheStats{}
	}
	return b.cache.Stats()
}

// ClearCache removes all local prompt cache entries.
func (b *Builder) ClearCache() {
	if b == nil || b.cache == nil {
		return
	}
	b.cache.Clear()
}

func buildCacheKey(input BuildInput, memoryContext, summary string, history []llm.Message) string {
	var builder strings.Builder
	builder.WriteString(input.Username)
	builder.WriteString("\n")
	builder.WriteString(input.Model)
	builder.WriteString("\n")
	builder.WriteString(input.Prompt)
	builder.WriteString("\n")
	builder.WriteString(memoryContext)
	builder.WriteString("\n")
	builder.WriteString(summary)
	builder.WriteString("\n")
	for _, item := range history {
		builder.WriteString(item.Role)
		builder.WriteString(":")
		builder.WriteString(item.Content)
		builder.WriteString("\n")
	}
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func recentStoreMessagesToHistory(messages []store.Message) []llm.Message {
	history := make([]llm.Message, 0, len(messages))
	for _, msg := range messages {
		history = append(history, llm.NewMessage(msg.Role, msg.Content))
	}
	return history
}

func trimHistoryToBudget(systemBlocks []string, history []llm.Message, prompt string, maxChars int) []llm.Message {
	if maxChars <= 0 {
		return history
	}
	total := len(prompt) + len("You are a secure, concise assistant.")
	for _, block := range systemBlocks {
		total += len(block)
	}
	trimmed := append([]llm.Message(nil), history...)
	for _, item := range trimmed {
		total += len(item.Content)
	}
	for total > maxChars && len(trimmed) > 0 {
		total -= len(trimmed[0].Content)
		trimmed = trimmed[1:]
	}
	return trimmed
}

// FormatSummary returns a compact human-readable description of a prompt plan.
func (r BuildResult) FormatSummary() string {
	return fmt.Sprintf("model=%s history=%d/%d blocks=%d prompt_chars=%d/%d cache_hit=%t", r.Model, r.HistoryMessagesUsed, r.HistoryWindowMessages, r.SystemBlocksUsed, r.PromptChars, r.MaxPromptChars, r.CacheHit)
}
