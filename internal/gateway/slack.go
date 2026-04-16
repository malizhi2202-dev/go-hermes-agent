package gateway

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"hermes-agent/go/internal/app"
)

// SlackAdapter handles Slack slash command ingress with signature verification.
type SlackAdapter struct {
	app    *app.App
	chatFn func(ctx context.Context, username, prompt string) (string, error)
}

// NewSlackAdapter creates a Slack gateway adapter.
func NewSlackAdapter(application *app.App) *SlackAdapter {
	return &SlackAdapter{
		app:    application,
		chatFn: application.Chat,
	}
}

// HandleCommand validates Slack signatures, routes the command into chat or
// multi-agent execution, and returns an immediate slash-command response.
func (a *SlackAdapter) HandleCommand(w http.ResponseWriter, r *http.Request) {
	cfg := a.app.Config.Gateway.Slack
	if !cfg.Enabled {
		http.Error(w, "slack gateway disabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rawBody, err := readRequestBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	if !a.verifySignature(r, rawBody) {
		http.Error(w, "invalid slack signature", http.StatusUnauthorized)
		return
	}
	values, err := url.ParseQuery(string(rawBody))
	if err != nil {
		http.Error(w, "invalid form body", http.StatusBadRequest)
		return
	}
	text := strings.TrimSpace(values.Get("text"))
	channelID := strings.TrimSpace(values.Get("channel_id"))
	userID := strings.TrimSpace(values.Get("user_id"))
	username := strings.TrimSpace(values.Get("user_name"))
	if username == "" {
		username = userID
	}
	if username == "" || text == "" {
		http.Error(w, "user_name/user_id and text are required", http.StatusBadRequest)
		return
	}
	sessionPrincipal := fmt.Sprintf("slack:channel:%s:user:%s", channelID, userID)
	if channelID == "" {
		sessionPrincipal = "slack:user:" + userID
	}
	response, err := a.routeMessage(r.Context(), sessionPrincipal, text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = a.app.Store.WriteAudit(r.Context(), sessionPrincipal, "gateway_slack_command", fmt.Sprintf("display=%s channel_id=%s at=%s", username, channelID, time.Now().UTC().Format(time.RFC3339)))
	writeJSON(w, http.StatusOK, map[string]any{
		"response_type": "ephemeral",
		"text":          response,
	})
}

func (a *SlackAdapter) routeMessage(ctx context.Context, username, text string) (string, error) {
	if objective, ok := parseMultiAgentCommand(text); ok {
		return a.app.RunGatewayMultiAgent(ctx, username, objective)
	}
	return a.chatFn(ctx, username, text)
}

func (a *SlackAdapter) verifySignature(r *http.Request, rawBody []byte) bool {
	secret := strings.TrimSpace(os.Getenv(a.app.Config.Gateway.Slack.SigningSecretEnv))
	if secret == "" {
		return false
	}
	timestamp := strings.TrimSpace(r.Header.Get("X-Slack-Request-Timestamp"))
	signature := strings.TrimSpace(r.Header.Get("X-Slack-Signature"))
	if timestamp == "" || signature == "" {
		return false
	}
	sec, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	ts := time.Unix(sec, 0)
	if delta := time.Since(ts); delta > 5*time.Minute || delta < -5*time.Minute {
		return false
	}
	base := "v0:" + timestamp + ":" + string(rawBody)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(base))
	expected := "v0=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func readRequestBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	return io.ReadAll(r.Body)
}
