package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

func slackSignature(secret string, timestamp int64, body string) string {
	base := "v0:" + strconv.FormatInt(timestamp, 10) + ":" + body
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	return "v0=" + hex.EncodeToString(mac.Sum(nil))
}
