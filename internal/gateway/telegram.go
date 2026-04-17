package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
)

// TelegramAdapter handles Telegram webhook ingress and reply delivery.
//
// It supports deduplication, reply retries, chat/user session isolation, and a
// lightweight `/multiagent ...` route into the Go multi-agent runtime.
type TelegramAdapter struct {
	app        *app.App
	httpClient *http.Client
	apiBaseURL string
	chatFn     func(ctx context.Context, username, prompt string) (string, error)
}

type telegramUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64  `json:"message_id"`
		Text      string `json:"text"`
		Chat      struct {
			ID int64 `json:"id"`
		} `json:"chat"`
		From struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
		} `json:"from"`
	} `json:"message"`
}

// NewTelegramAdapter creates a Telegram webhook adapter.
func NewTelegramAdapter(application *app.App) *TelegramAdapter {
	return &TelegramAdapter{
		app:        application,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		apiBaseURL: "https://api.telegram.org",
		chatFn:     application.Chat,
	}
}

// Name returns the adapter name.
func (a *TelegramAdapter) Name() string {
	return "telegram"
}

// Routes returns the inbound routes exposed by the Telegram adapter.
func (a *TelegramAdapter) Routes() []Route {
	return []Route{{Path: "/gateway/telegram/webhook", Handler: a.HandleWebhook}}
}

// HandleWebhook validates the Telegram secret, deduplicates updates, routes
// the message into chat or multi-agent execution, and sends the reply.
func (a *TelegramAdapter) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := a.app.Config.Gateway.Telegram
	if !cfg.Enabled {
		http.Error(w, "telegram gateway disabled", http.StatusNotFound)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if strings.TrimSpace(r.Header.Get("X-Telegram-Bot-Api-Secret-Token")) != cfg.Secret {
		http.Error(w, "invalid telegram secret", http.StatusUnauthorized)
		return
	}
	var update telegramUpdate
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if update.Message == nil || strings.TrimSpace(update.Message.Text) == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ignored": true})
		return
	}

	externalID := fmt.Sprintf("%d", update.UpdateID)
	if externalID == "0" {
		externalID = fmt.Sprintf("%d:%d", update.Message.Chat.ID, update.Message.MessageID)
	}
	inserted, err := a.app.Store.MarkGatewayUpdateProcessed(r.Context(), "telegram", externalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !inserted {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "duplicate": true})
		return
	}

	displayName := update.Message.From.Username
	if strings.TrimSpace(displayName) == "" {
		displayName = fmt.Sprintf("telegram:%d", update.Message.From.ID)
	}
	sessionPrincipal := fmt.Sprintf("telegram:chat:%d:user:%d", update.Message.Chat.ID, update.Message.From.ID)
	response, err := a.routeMessage(context.Background(), sessionPrincipal, update.Message.Text)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.sendMessage(r.Context(), update.Message.Chat.ID, response); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	_ = a.app.Store.WriteAudit(r.Context(), sessionPrincipal, "gateway_telegram_message", fmt.Sprintf("display=%s chat_id=%d update_id=%s at=%s", displayName, update.Message.Chat.ID, externalID, time.Now().UTC().Format(time.RFC3339)))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *TelegramAdapter) sendMessage(ctx context.Context, chatID int64, text string) error {
	token := strings.TrimSpace(os.Getenv(a.app.Config.Gateway.Telegram.BotTokenEnv))
	if token == "" {
		return fmt.Errorf("missing telegram bot token env %q", a.app.Config.Gateway.Telegram.BotTokenEnv)
	}
	body, err := json.Marshal(map[string]any{
		"chat_id": chatID,
		"text":    text,
	})
	if err != nil {
		return err
	}

	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiBaseURL+"/bot"+token+"/sendMessage", bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := a.httpClient.Do(req)
		if err == nil {
			responseBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("telegram sendMessage failed with status %s: %s", resp.Status, strings.TrimSpace(string(responseBody)))
		} else {
			lastErr = err
		}
		time.Sleep(time.Duration(attempt+1) * 300 * time.Millisecond)
	}
	return lastErr
}

func (a *TelegramAdapter) routeMessage(ctx context.Context, username, text string) (string, error) {
	if objective, ok := parseMultiAgentCommand(text); ok {
		return a.app.RunGatewayMultiAgent(ctx, username, objective)
	}
	return a.chatFn(ctx, username, text)
}
