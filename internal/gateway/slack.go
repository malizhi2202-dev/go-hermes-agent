package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
)

// SlackAdapter handles Slack slash command ingress with signature verification.
type SlackAdapter struct {
	app        *app.App
	httpClient *http.Client
	apiBaseURL string
	chatFn     func(ctx context.Context, username, prompt string) (string, error)
}

// NewSlackAdapter creates a Slack gateway adapter.
func NewSlackAdapter(application *app.App) *SlackAdapter {
	return &SlackAdapter{
		app:        application,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBaseURL: "https://slack.com/api",
		chatFn:     application.Chat,
	}
}

// Name returns the adapter name.
func (a *SlackAdapter) Name() string {
	return "slack"
}

// Routes returns the inbound routes exposed by the Slack adapter.
func (a *SlackAdapter) Routes() []Route {
	return []Route{
		{Path: "/gateway/slack/command", Handler: a.HandleCommand},
		{Path: "/gateway/slack/events", Handler: a.HandleEvents},
	}
}

type slackEventEnvelope struct {
	Type      string `json:"type"`
	Challenge string `json:"challenge"`
	EventID   string `json:"event_id"`
	EventTime int64  `json:"event_time"`
	Event     *struct {
		Type     string `json:"type"`
		Subtype  string `json:"subtype"`
		Text     string `json:"text"`
		User     string `json:"user"`
		Channel  string `json:"channel"`
		ThreadTS string `json:"thread_ts"`
		TS       string `json:"ts"`
		BotID    string `json:"bot_id"`
	} `json:"event"`
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

// HandleEvents validates Slack signatures, handles URL verification, routes
// app events into chat or multi-agent execution, deduplicates updates, and
// sends replies through chat.postMessage.
func (a *SlackAdapter) HandleEvents(w http.ResponseWriter, r *http.Request) {
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
	var envelope slackEventEnvelope
	if err := json.Unmarshal(rawBody, &envelope); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if envelope.Type == "url_verification" {
		writeJSON(w, http.StatusOK, map[string]any{"challenge": envelope.Challenge})
		return
	}
	if envelope.Type != "event_callback" || envelope.Event == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}
	if envelope.Event.BotID != "" || strings.TrimSpace(envelope.Event.Subtype) == "bot_message" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}
	externalID := strings.TrimSpace(envelope.EventID)
	if externalID == "" {
		externalID = strings.TrimSpace(envelope.Event.TS)
	}
	if externalID != "" {
		inserted, err := a.app.Store.MarkGatewayUpdateProcessed(r.Context(), "slack", externalID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if !inserted {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true})
			return
		}
	}
	userID := strings.TrimSpace(envelope.Event.User)
	channelID := strings.TrimSpace(envelope.Event.Channel)
	text := normalizeSlackText(envelope.Event.Text)
	if userID == "" || channelID == "" || text == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}
	sessionPrincipal := fmt.Sprintf("slack:channel:%s:user:%s", channelID, userID)
	response, err := a.routeMessage(r.Context(), sessionPrincipal, text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.sendMessage(r.Context(), channelID, firstNonEmpty(envelope.Event.ThreadTS, envelope.Event.TS), response); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = a.app.Store.WriteAudit(r.Context(), sessionPrincipal, "gateway_slack_event", fmt.Sprintf("channel_id=%s event_id=%s at=%s", channelID, externalID, time.Now().UTC().Format(time.RFC3339)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *SlackAdapter) routeMessage(ctx context.Context, username, text string) (string, error) {
	if objective, ok := parseMultiAgentCommand(text); ok {
		return a.app.RunGatewayMultiAgent(ctx, username, objective)
	}
	return a.chatFn(ctx, username, text)
}

func (a *SlackAdapter) sendMessage(ctx context.Context, channelID, threadTS, text string) error {
	token := strings.TrimSpace(os.Getenv(a.app.Config.Gateway.Slack.BotTokenEnv))
	if token == "" {
		return fmt.Errorf("missing slack bot token env %q", a.app.Config.Gateway.Slack.BotTokenEnv)
	}
	payload := map[string]any{
		"channel": channelID,
		"text":    text,
	}
	if strings.TrimSpace(threadTS) != "" {
		payload["thread_ts"] = threadTS
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiBaseURL+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("slack chat.postMessage failed with status %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	var payloadResp struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &payloadResp); err != nil {
		return err
	}
	if !payloadResp.OK {
		return fmt.Errorf("slack chat.postMessage error: %s", firstNonEmpty(payloadResp.Error, "unknown_error"))
	}
	return nil
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

func normalizeSlackText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) > 0 && strings.HasPrefix(fields[0], "<@") && strings.HasSuffix(fields[0], ">") {
		return strings.TrimSpace(strings.Join(fields[1:], " "))
	}
	return text
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
