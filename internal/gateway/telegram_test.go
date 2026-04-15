package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/config"
)

func TestTelegramWebhookDedupesUpdates(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Gateway.Telegram.Enabled = true
	cfg.Gateway.Telegram.Secret = "secret"
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	adapter := NewTelegramAdapter(application)
	adapter.chatFn = func(_ context.Context, username, prompt string) (string, error) {
		return "reply:" + username + ":" + prompt, nil
	}
	adapter.httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		rec := httptest.NewRecorder()
		rec.WriteHeader(http.StatusOK)
		_, _ = rec.WriteString(`{"ok":true}`)
		return rec.Result(), nil
	})}
	t.Setenv("TELEGRAM_BOT_TOKEN", "token")

	body := `{"update_id":1001,"message":{"message_id":1,"text":"hi","chat":{"id":123},"from":{"id":55,"username":"alice"}}}`
	req1 := httptest.NewRequest(http.MethodPost, "/gateway/telegram/webhook", strings.NewReader(body))
	req1.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec1 := httptest.NewRecorder()
	adapter.HandleWebhook(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request expected 200, got %d", rec1.Code)
	}

	req2 := httptest.NewRequest(http.MethodPost, "/gateway/telegram/webhook", strings.NewReader(body))
	req2.Header.Set("X-Telegram-Bot-Api-Secret-Token", "secret")
	rec2 := httptest.NewRecorder()
	adapter.HandleWebhook(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("second request expected 200, got %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate response, got %s", rec2.Body.String())
	}
}

type roundTripperFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}
