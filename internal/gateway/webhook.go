package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
)

type WebhookAdapter struct {
	app *app.App
}

type InboundMessage struct {
	Platform string `json:"platform"`
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Text     string `json:"text"`
}

func NewWebhookAdapter(application *app.App) *WebhookAdapter {
	return &WebhookAdapter{app: application}
}

func (a *WebhookAdapter) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !a.authorize(r) {
		http.Error(w, "invalid gateway token", http.StatusUnauthorized)
		return
	}
	var input InboundMessage
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(input.Platform) == "" {
		input.Platform = "webhook"
	}
	if strings.TrimSpace(input.Username) == "" {
		input.Username = strings.TrimSpace(input.UserID)
	}
	if strings.TrimSpace(input.Username) == "" || strings.TrimSpace(input.Text) == "" {
		http.Error(w, "username/user_id and text are required", http.StatusBadRequest)
		return
	}
	response, err := a.app.Chat(context.Background(), input.Username, input.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = a.app.Store.WriteAudit(r.Context(), input.Username, "gateway_webhook_message", fmt.Sprintf("platform=%s at=%s", input.Platform, time.Now().UTC().Format(time.RFC3339)))
	writeJSON(w, http.StatusOK, map[string]any{
		"platform": input.Platform,
		"user":     input.Username,
		"reply":    response,
	})
}

func (a *WebhookAdapter) authorize(r *http.Request) bool {
	token := strings.TrimSpace(r.Header.Get("X-Gateway-Token"))
	return token != "" && token == a.app.Config.Gateway.Token
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
