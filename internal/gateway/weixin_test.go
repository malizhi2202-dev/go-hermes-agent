package gateway

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go-hermes-agent/internal/app"
	"go-hermes-agent/internal/config"
)

func TestWeixinHandshakeAndDedupedTextRouting(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.Gateway.Weixin.Enabled = true
	cfg.Gateway.Weixin.Token = "secret-token"
	application, err := app.New(cfg)
	if err != nil {
		t.Fatalf("init app: %v", err)
	}
	defer application.Close()

	seedSession, err := application.Store.CreateSession(context.Background(), "weixin:alice", "model", "seed", "seed")
	if err != nil {
		t.Fatalf("seed session: %v", err)
	}
	if err := application.Store.AddMessage(context.Background(), seedSession, "user", "alpha"); err != nil {
		t.Fatalf("seed message: %v", err)
	}

	adapter := NewWeixinAdapter(application)
	adapter.chatFn = func(_ context.Context, username, prompt string) (string, error) {
		return "reply:" + username + ":" + prompt, nil
	}

	timestamp := "1713340800"
	nonce := "xyz"
	signature := weixinSignature(cfg.Gateway.Weixin.Token, timestamp, nonce)
	getReq := httptest.NewRequest(http.MethodGet, "/gateway/weixin/webhook?timestamp="+timestamp+"&nonce="+nonce+"&signature="+signature+"&echostr=ok", nil)
	getRec := httptest.NewRecorder()
	adapter.HandleWebhook(getRec, getReq)
	if getRec.Code != http.StatusOK || strings.TrimSpace(getRec.Body.String()) != "ok" {
		t.Fatalf("unexpected handshake response: code=%d body=%s", getRec.Code, getRec.Body.String())
	}

	body := `<xml><ToUserName><![CDATA[gh_1]]></ToUserName><FromUserName><![CDATA[alice]]></FromUserName><CreateTime>1713340800</CreateTime><MsgType><![CDATA[text]]></MsgType><Content><![CDATA[/multiagent inspect prior work]]></Content><MsgId>9001</MsgId></xml>`
	postReq := httptest.NewRequest(http.MethodPost, "/gateway/weixin/webhook?timestamp="+timestamp+"&nonce="+nonce+"&signature="+signature, strings.NewReader(body))
	postRec := httptest.NewRecorder()
	adapter.HandleWebhook(postRec, postReq)
	if postRec.Code != http.StatusOK {
		t.Fatalf("unexpected post response: code=%d body=%s", postRec.Code, postRec.Body.String())
	}
	if !strings.Contains(postRec.Body.String(), "[multiagent aggregate]") {
		t.Fatalf("expected multiagent reply, got %s", postRec.Body.String())
	}

	dupReq := httptest.NewRequest(http.MethodPost, "/gateway/weixin/webhook?timestamp="+timestamp+"&nonce="+nonce+"&signature="+signature, strings.NewReader(body))
	dupRec := httptest.NewRecorder()
	adapter.HandleWebhook(dupRec, dupReq)
	if dupRec.Code != http.StatusOK || strings.TrimSpace(dupRec.Body.String()) != "success" {
		t.Fatalf("unexpected duplicate response: code=%d body=%s", dupRec.Code, dupRec.Body.String())
	}
}

func weixinSignature(token, timestamp, nonce string) string {
	parts := []string{token, timestamp, nonce}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(sum[:])
}
