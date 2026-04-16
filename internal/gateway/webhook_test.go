package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"hermes-agent/go/internal/app"
	"hermes-agent/go/internal/config"
)

func TestWebhookRoutesMultiAgentCommand(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Gateway.Token = "secret-token"
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	seedSession, err := application.Store.CreateSession(context.Background(), "alice", "model", "seed", "seed")
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := application.Store.AddMessage(context.Background(), seedSession, "user", "alpha"); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	adapter := NewWebhookAdapter(application)
	body := []byte(`{"platform":"webhook","username":"alice","text":"/multiagent inspect prior work"}`)
	req := httptest.NewRequest(http.MethodPost, "/gateway/webhook", bytes.NewReader(body))
	req.Header.Set("X-Gateway-Token", "secret-token")
	rec := httptest.NewRecorder()
	adapter.HandleWebhook(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	reply, _ := payload["reply"].(string)
	if !strings.Contains(reply, "[multiagent aggregate]") {
		t.Fatalf("expected multiagent aggregate reply, got %#v", payload)
	}
}
