package tools

import (
	"context"
	"testing"
)

func TestRegistryRegisterListAndExecute(t *testing.T) {
	reg := New()
	err := reg.Register(Tool{
		Name:        "test.echo",
		Description: "echo input",
		InputKeys:   []string{"value"},
		Handler: func(_ context.Context, input map[string]any) (map[string]any, error) {
			return map[string]any{"value": input["value"]}, nil
		},
	})
	if err != nil {
		t.Fatalf("register tool: %v", err)
	}
	tools := reg.List()
	if len(tools) != 1 || tools[0].Name != "test.echo" {
		t.Fatalf("unexpected tools list: %#v", tools)
	}
	result, err := reg.Execute(context.Background(), "test.echo", map[string]any{"value": "ok"})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if result["value"] != "ok" {
		t.Fatalf("unexpected tool result: %#v", result)
	}
}
