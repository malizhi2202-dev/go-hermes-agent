package gateway

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go-hermes-agent/internal/app"
)

// WeixinAdapter handles lightweight Weixin public-account style webhook ingress.
//
// The adapter intentionally supports only plaintext signature verification and
// text-message roundtrips so the Go edition stays easy to deploy and extend.
type WeixinAdapter struct {
	app    *app.App
	chatFn func(ctx context.Context, username, prompt string) (string, error)
}

type weixinInboundMessage struct {
	XMLName      xml.Name `xml:"xml"`
	ToUserName   string   `xml:"ToUserName"`
	FromUserName string   `xml:"FromUserName"`
	CreateTime   int64    `xml:"CreateTime"`
	MsgType      string   `xml:"MsgType"`
	Content      string   `xml:"Content"`
	MsgID        int64    `xml:"MsgId"`
	Event        string   `xml:"Event"`
}

// NewWeixinAdapter creates a Weixin webhook adapter.
func NewWeixinAdapter(application *app.App) *WeixinAdapter {
	return &WeixinAdapter{
		app:    application,
		chatFn: application.Chat,
	}
}

// Name returns the adapter name.
func (a *WeixinAdapter) Name() string {
	return "weixin"
}

// Routes returns the inbound routes exposed by the Weixin adapter.
func (a *WeixinAdapter) Routes() []Route {
	return []Route{{Path: "/gateway/weixin/webhook", Handler: a.HandleWebhook}}
}

// HandleWebhook verifies the Weixin signature, completes the handshake, and
// routes text messages into chat or the multi-agent path.
func (a *WeixinAdapter) HandleWebhook(w http.ResponseWriter, r *http.Request) {
	cfg := a.app.Config.Gateway.Weixin
	if !cfg.Enabled {
		http.Error(w, "weixin gateway disabled", http.StatusNotFound)
		return
	}
	if !a.verifySignature(r) {
		http.Error(w, "invalid weixin signature", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(r.URL.Query().Get("echostr")))
		return
	case http.MethodPost:
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := readRequestBody(r)
	if err != nil {
		http.Error(w, "read body failed", http.StatusBadRequest)
		return
	}
	var message weixinInboundMessage
	if err := xml.Unmarshal(body, &message); err != nil {
		http.Error(w, "invalid xml", http.StatusBadRequest)
		return
	}
	msgType := strings.ToLower(strings.TrimSpace(message.MsgType))
	if msgType != "text" || strings.TrimSpace(message.Content) == "" {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("success"))
		return
	}

	sessionPrincipal := fmt.Sprintf("weixin:%s", strings.TrimSpace(message.FromUserName))
	response, err := a.routeMessage(r.Context(), sessionPrincipal, strings.TrimSpace(message.Content))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	externalID := strconv.FormatInt(message.MsgID, 10)
	if externalID == "0" {
		externalID = fmt.Sprintf("%s-%d", sessionPrincipal, time.Now().UTC().UnixNano())
	}
	marked, err := a.app.Store.MarkGatewayUpdateProcessed(r.Context(), "weixin", externalID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !marked {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("success"))
		return
	}
	_ = a.app.Store.WriteAudit(r.Context(), sessionPrincipal, "gateway_weixin_message", fmt.Sprintf("from=%s msg_id=%s at=%s", message.FromUserName, externalID, time.Now().UTC().Format(time.RFC3339)))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, _ = w.Write(renderWeixinTextReply(message.FromUserName, message.ToUserName, response))
}

func (a *WeixinAdapter) routeMessage(ctx context.Context, username, text string) (string, error) {
	if objective, ok := parseMultiAgentCommand(text); ok {
		return a.app.RunGatewayMultiAgent(ctx, username, objective)
	}
	return a.chatFn(ctx, username, text)
}

func (a *WeixinAdapter) verifySignature(r *http.Request) bool {
	token := strings.TrimSpace(a.app.Config.Gateway.Weixin.Token)
	timestamp := strings.TrimSpace(r.URL.Query().Get("timestamp"))
	nonce := strings.TrimSpace(r.URL.Query().Get("nonce"))
	signature := strings.TrimSpace(r.URL.Query().Get("signature"))
	if token == "" || timestamp == "" || nonce == "" || signature == "" {
		return false
	}
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return strings.EqualFold(signature, hex.EncodeToString(sum[:]))
}

func renderWeixinTextReply(toUser, fromUser, content string) []byte {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(content))
	reply := fmt.Sprintf(
		"<xml><ToUserName><![CDATA[%s]]></ToUserName><FromUserName><![CDATA[%s]]></FromUserName><CreateTime>%d</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[%s]]></Content></xml>",
		toUser,
		fromUser,
		time.Now().UTC().Unix(),
		escaped.String(),
	)
	return []byte(reply)
}
