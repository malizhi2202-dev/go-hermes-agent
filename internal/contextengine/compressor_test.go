package contextengine

import (
	"context"
	"strings"
	"testing"

	"go-hermes-agent/internal/config"
	"go-hermes-agent/internal/llm"
)

func TestCompressorCompressesOlderMessagesAndKeepsTail(t *testing.T) {
	compressor := New(config.ContextConfig{
		CompressionEnabled:        true,
		CompressThresholdMessages: 4,
		ProtectLastMessages:       2,
		SummaryMaxChars:           400,
	})
	history := []llm.Message{
		llm.NewMessage("user", "first user request"),
		llm.NewMessage("assistant", "first assistant reply"),
		llm.NewMessage("user", "second user request"),
		llm.NewMessage("assistant", "second assistant reply"),
		llm.NewMessage("user", "latest question"),
	}

	result := compressor.Compress(context.Background(), "", history)
	if !result.Compressed {
		t.Fatal("expected compression to happen")
	}
	if result.CompressedMessages != 3 {
		t.Fatalf("expected 3 compressed messages, got %d", result.CompressedMessages)
	}
	if len(result.History) != 2 {
		t.Fatalf("expected tail history length 2, got %d", len(result.History))
	}
	if result.History[0].Content != "second assistant reply" || result.History[1].Content != "latest question" {
		t.Fatalf("unexpected tail history: %#v", result.History)
	}
	if !strings.Contains(result.SystemBlock, "Compressed 3 earlier messages") {
		t.Fatalf("unexpected system block: %s", result.SystemBlock)
	}
}

func TestCompressorLeavesHistoryWhenDisabled(t *testing.T) {
	compressor := New(config.ContextConfig{
		CompressionEnabled: false,
	})
	history := []llm.Message{
		llm.NewMessage("user", "hello"),
		llm.NewMessage("assistant", "world"),
	}
	result := compressor.Compress(context.Background(), "", history)
	if result.Compressed {
		t.Fatal("expected no compression")
	}
	if len(result.History) != 2 {
		t.Fatalf("expected unchanged history, got %d", len(result.History))
	}
}

func TestCompressorCarriesExistingSummaryForward(t *testing.T) {
	compressor := New(config.ContextConfig{
		CompressionEnabled:        true,
		CompressThresholdMessages: 2,
		ProtectLastMessages:       1,
		SummaryMaxChars:           500,
		SummaryStrategy:           "rule",
	})
	history := []llm.Message{
		llm.NewMessage("user", "new request"),
		llm.NewMessage("assistant", "new reply"),
		llm.NewMessage("user", "latest"),
	}
	result := compressor.Compress(context.Background(), "Earlier summary block", history)
	if !strings.Contains(result.PersistedSummary, "Earlier summary block") {
		t.Fatalf("expected existing summary to be carried forward: %s", result.PersistedSummary)
	}
}

func TestCompressorUsesLLMSummarizerWhenConfigured(t *testing.T) {
	compressor := New(config.ContextConfig{
		CompressionEnabled:        true,
		CompressThresholdMessages: 2,
		ProtectLastMessages:       1,
		SummaryMaxChars:           200,
		SummaryStrategy:           "llm",
	}).WithSummarizer(func(_ context.Context, existingSummary string, history []llm.Message, maxChars int) (string, error) {
		return "llm-summary", nil
	})
	history := []llm.Message{
		llm.NewMessage("user", "a"),
		llm.NewMessage("assistant", "b"),
		llm.NewMessage("user", "c"),
	}
	result := compressor.Compress(context.Background(), "old", history)
	if result.PersistedSummary != "llm-summary" {
		t.Fatalf("expected llm summary, got %q", result.PersistedSummary)
	}
}
