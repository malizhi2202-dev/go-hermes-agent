package execution

import (
	"context"
	"testing"

	"hermes-agent/go/internal/config"
)

func TestExecutorBlocksForbiddenArgs(t *testing.T) {
	exec := NewExecutor(config.ExecutionConfig{
		Enabled:         true,
		TimeoutSeconds:  5,
		AllowedCommands: []string{"echo"},
		MaxArgs:         4,
		MaxArgLength:    64,
		MaxOutputBytes:  256,
	})
	_, err := exec.Execute(context.Background(), "echo", []string{"hello;rm -rf /"})
	if err == nil {
		t.Fatal("expected forbidden arg validation error")
	}
}

func TestExecutorRunsAllowedCommand(t *testing.T) {
	exec := NewExecutor(config.ExecutionConfig{
		Enabled:         true,
		TimeoutSeconds:  5,
		AllowedCommands: []string{"echo"},
		MaxArgs:         4,
		MaxArgLength:    64,
		MaxOutputBytes:  256,
	})
	output, err := exec.Execute(context.Background(), "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if output != "hello" {
		t.Fatalf("unexpected output: %q", output)
	}
}

func TestExecutorAppliesPerCommandPrefixRules(t *testing.T) {
	exec := NewExecutor(config.ExecutionConfig{
		Enabled:         true,
		TimeoutSeconds:  5,
		AllowedCommands: []string{"date"},
		MaxArgs:         4,
		MaxArgLength:    64,
		MaxOutputBytes:  256,
		CommandRules: map[string]config.CommandRule{
			"date": {
				MaxArgs:            2,
				MaxArgLength:       64,
				MaxOutputBytes:     256,
				AllowedArgPrefixes: []string{"+"},
			},
		},
	})
	if _, err := exec.Execute(context.Background(), "date", []string{"-u"}); err == nil {
		t.Fatal("expected prefix rule rejection")
	}
}
