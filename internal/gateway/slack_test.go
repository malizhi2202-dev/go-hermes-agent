package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"hermes-agent/go/internal/app"
	"hermes-agent/go/internal/config"
)

func TestSlackCommandVerifiesSignatureAndRoutesMultiAgent(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Gateway.Slack.Enabled = true
	cfg.Gateway.Slack.SigningSecretEnv = "SLACK_SIGNING_SECRET"
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	adapter := NewSlackAdapter(application)
	adapter.chatFn = func(_ context.Context, username, prompt string) (string, error) {
		return "reply:" + username + ":" + prompt, nil
	}
	t.Setenv("SLACK_SIGNING_SECRET", "secret")

	seedSession, err := application.Store.CreateSession(context.Background(), "slack:channel:C1:user:U1", "model", "seed", "seed")
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := application.Store.AddMessage(context.Background(), seedSession, "user", "alpha"); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	body := "command=%2Fhermes&text=%2Fmultiagent+inspect+history&user_id=U1&user_name=alice&channel_id=C1"
	req := httptest.NewRequest(http.MethodPost, "/gateway/slack/command", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	timestamp := time.Now().UTC().Unix()
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Slack-Signature", slackSignature("secret", timestamp, body))
	rec := httptest.NewRecorder()
	adapter.HandleCommand(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "[multiagent aggregate]") {
		t.Fatalf("expected multiagent response, got %s", rec.Body.String())
	}
}

func TestSlackEventsVerifyAndReply(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Gateway.Slack.Enabled = true
	cfg.Gateway.Slack.SigningSecretEnv = "SLACK_SIGNING_SECRET"
	cfg.Gateway.Slack.BotTokenEnv = "SLACK_BOT_TOKEN"
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	var posted map[string]any
	slackAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.URL.Path != "/chat.postMessage" {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bot-token" {
			t.Fatalf("unexpected auth header: %s", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&posted); err != nil {
			t.Fatalf("decode slack api body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer slackAPI.Close()

	adapter := NewSlackAdapter(application)
	adapter.apiBaseURL = slackAPI.URL
	adapter.chatFn = func(_ context.Context, username, prompt string) (string, error) {
		return "reply:" + username + ":" + prompt, nil
	}
	t.Setenv("SLACK_SIGNING_SECRET", "secret")
	t.Setenv("SLACK_BOT_TOKEN", "bot-token")

	payload := map[string]any{
		"type":       "event_callback",
		"event_id":   "Ev123",
		"event_time": time.Now().UTC().Unix(),
		"event": map[string]any{
			"type":    "app_mention",
			"text":    "<@B1> hello there",
			"user":    "U2",
			"channel": "C2",
			"ts":      "1710000000.123",
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/gateway/slack/events", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	timestamp := time.Now().UTC().Unix()
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Slack-Signature", slackSignature("secret", timestamp, string(raw)))
	rec := httptest.NewRecorder()
	adapter.HandleEvents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if posted["channel"] != "C2" || posted["thread_ts"] != "1710000000.123" {
		t.Fatalf("unexpected posted payload: %#v", posted)
	}
	if posted["text"] != "reply:slack:channel:C2:user:U2:hello there" {
		t.Fatalf("unexpected posted text: %#v", posted)
	}

	dupReq := httptest.NewRequest(http.MethodPost, "/gateway/slack/events", bytes.NewReader(raw))
	dupReq.Header.Set("Content-Type", "application/json")
	dupReq.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(timestamp, 10))
	dupReq.Header.Set("X-Slack-Signature", slackSignature("secret", timestamp, string(raw)))
	dupRec := httptest.NewRecorder()
	adapter.HandleEvents(dupRec, dupReq)
	if dupRec.Code != http.StatusOK || !strings.Contains(dupRec.Body.String(), `"duplicate":true`) {
		t.Fatalf("expected duplicate response, got %d body=%s", dupRec.Code, dupRec.Body.String())
	}
}

func TestSlackEventsURLVerification(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Gateway.Slack.Enabled = true
	cfg.Gateway.Slack.SigningSecretEnv = "SLACK_SIGNING_SECRET"
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()
	adapter := NewSlackAdapter(application)
	t.Setenv("SLACK_SIGNING_SECRET", "secret")

	body := `{"type":"url_verification","challenge":"abc123"}`
	req := httptest.NewRequest(http.MethodPost, "/gateway/slack/events", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	timestamp := time.Now().UTC().Unix()
	req.Header.Set("X-Slack-Request-Timestamp", strconv.FormatInt(timestamp, 10))
	req.Header.Set("X-Slack-Signature", slackSignature("secret", timestamp, body))
	rec := httptest.NewRecorder()
	adapter.HandleEvents(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "abc123") {
		t.Fatalf("expected challenge response, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func slackSignature(secret string, timestamp int64, body string) string {
	base := "v0:" + strconv.FormatInt(timestamp, 10) + ":" + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}
