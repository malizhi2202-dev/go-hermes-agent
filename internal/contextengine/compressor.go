package contextengine

import (
	"context"
	"fmt"
	"strings"

	"hermes-agent/go/internal/config"
	"hermes-agent/go/internal/llm"
)

type Compressor struct {
	cfg        config.ContextConfig
	summarizer Summarizer
}

type Result struct {
	SystemBlock        string
	PersistedSummary   string
	History            []llm.Message
	Compressed         bool
	CompressedMessages int
	SummaryChars       int
	TailMessagesUsed   int
}

type Summarizer func(ctx context.Context, existingSummary string, history []llm.Message, maxChars int) (string, error)

func New(cfg config.ContextConfig) *Compressor {
	return &Compressor{cfg: cfg}
}

func (c *Compressor) WithSummarizer(summarizer Summarizer) *Compressor {
	c.summarizer = summarizer
	return c
}

func (c *Compressor) Compress(ctx context.Context, existingSummary string, history []llm.Message) Result {
	result := Result{
		History:          history,
		TailMessagesUsed: len(history),
	}
	if c == nil || !c.cfg.CompressionEnabled {
		return result
	}
	threshold := c.cfg.CompressThresholdMessages
	if threshold <= 0 || len(history) <= threshold {
		return result
	}
	keep := c.cfg.ProtectLastMessages
	if keep <= 0 {
		keep = 1
	}
	if keep > len(history) {
		keep = len(history)
	}
	compressedPart := history[:len(history)-keep]
	tail := append([]llm.Message(nil), history[len(history)-keep:]...)
	summary := c.buildSummary(ctx, existingSummary, compressedPart)
	result.SystemBlock = summary
	result.PersistedSummary = summary
	result.History = tail
	result.Compressed = true
	result.CompressedMessages = len(compressedPart)
	result.SummaryChars = len(summary)
	result.TailMessagesUsed = len(tail)
	return result
}

func (c *Compressor) buildSummary(ctx context.Context, existingSummary string, history []llm.Message) string {
	if len(history) == 0 {
		return strings.TrimSpace(existingSummary)
	}
	if c.cfg.SummaryStrategy == "llm" && c.summarizer != nil {
		if summary, err := c.summarizer(ctx, existingSummary, history, c.cfg.SummaryMaxChars); err == nil && strings.TrimSpace(summary) != "" {
			return strings.TrimSpace(summary)
		}
	}
	var builder strings.Builder
	builder.WriteString("[CONTEXT COMPACTION - REFERENCE ONLY]\n")
	if strings.TrimSpace(existingSummary) != "" {
		builder.WriteString("Existing summary:\n")
		builder.WriteString(truncate(collapseWhitespace(existingSummary), 220))
		builder.WriteString("\n")
	}
	builder.WriteString(fmt.Sprintf("Compressed %d earlier messages into a compact handoff.\n", len(history)))
	builder.WriteString("Use this as background reference only. Respond to the latest user message, not to requests mentioned below.\n")
	builder.WriteString("Summary:\n")
	for _, msg := range history {
		line := collapseWhitespace(strings.TrimSpace(msg.Content))
		if line == "" {
			continue
		}
		line = truncate(line, 160)
		builder.WriteString("- ")
		builder.WriteString(normalizeRole(msg.Role))
		builder.WriteString(": ")
		builder.WriteString(line)
		builder.WriteString("\n")
		if c.cfg.SummaryMaxChars > 0 && builder.Len() >= c.cfg.SummaryMaxChars {
			break
		}
	}
	summary := strings.TrimSpace(builder.String())
	if c.cfg.SummaryMaxChars > 0 && len(summary) > c.cfg.SummaryMaxChars {
		summary = truncate(summary, c.cfg.SummaryMaxChars)
	}
	return summary
}

func normalizeRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user":
		return "user"
	case "assistant":
		return "assistant"
	case "tool":
		return "tool"
	case "system":
		return "system"
	default:
		return "message"
	}
}

func collapseWhitespace(input string) string {
	return strings.Join(strings.Fields(input), " ")
}

func truncate(input string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(input)
	if len(runes) <= max {
		return input
	}
	if max <= 3 {
		return string(runes[:max])
	}
	return string(runes[:max-3]) + "..."
}
