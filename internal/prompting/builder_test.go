package prompting

import (
	"context"
	"testing"
	"time"

	"go-hermes-agent/internal/contextengine"
	"go-hermes-agent/internal/llm"
	"go-hermes-agent/internal/store"
)

func TestBuilderBuildAssemblesPromptPlan(t *testing.T) {
	builder := NewBuilder(Dependencies{
		PrefetchMemory: func(ctx context.Context, username, prompt string) (string, error) {
			return "memory block", nil
		},
		GetSummary: func(ctx context.Context, username string) (store.ContextSummary, error) {
			return store.ContextSummary{Username: username, Summary: "summary block", Strategy: "rule"}, nil
		},
		PersistSummary: func(ctx context.Context, username, summary, strategy string) error { return nil },
		ListRecent: func(ctx context.Context, username string, limit int) ([]store.Message, error) {
			return []store.Message{{Role: "user", Content: "alpha"}, {Role: "assistant", Content: "beta"}}, nil
		},
		Compress: func(ctx context.Context, existingSummary string, history []llm.Message) contextengine.Result {
			return contextengine.Result{History: history}
		},
	}, NewCache(time.Minute))
	result, err := builder.Build(context.Background(), BuildInput{Username: "alice", Prompt: "hello", Model: "test-model", HistoryWindow: 8, MaxPromptChars: 1000, SummaryStrategy: "rule", CompressionEnabled: true})
	if err != nil {
		t.Fatalf("build prompt: %v", err)
	}
	if result.Model != "test-model" || result.HistoryMessagesUsed != 2 || result.SystemBlocksUsed != 2 {
		t.Fatalf("unexpected build result: %#v", result)
	}
	cached, err := builder.Build(context.Background(), BuildInput{Username: "alice", Prompt: "hello", Model: "test-model", HistoryWindow: 8, MaxPromptChars: 1000, SummaryStrategy: "rule", CompressionEnabled: true})
	if err != nil {
		t.Fatalf("build cached prompt: %v", err)
	}
	if !cached.CacheHit {
		t.Fatalf("expected cache hit, got %#v", cached)
	}
}
