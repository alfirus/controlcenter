package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebhook_HMAC_Reject(t *testing.T) {
	h := &Handler{WebhookSecret: "s3cr3t"}
	body := `{"action":"opened"}`
	req := httptest.NewRequest("POST", "/v1/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", "sha256=invalid")
	rec := httptest.NewRecorder()
	h.webhook(rec, req)
	if rec.Code != 401 {
		t.Fatalf("expected 401 got %d", rec.Code)
	}
}

func TestWebhook_HMAC_Accept(t *testing.T) {
	secret := "s3cr3t"
	body := `{"action":"opened"}`
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	h := &Handler{WebhookSecret: secret}
	req := httptest.NewRequest("POST", "/v1/webhooks/github", strings.NewReader(body))
	req.Header.Set("X-Hub-Signature-256", sig)
	rec := httptest.NewRecorder()
	h.webhook(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestWebhook_NoSecret_Allows(t *testing.T) {
	h := &Handler{}
	req := httptest.NewRequest("POST", "/v1/webhooks/github", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.webhook(rec, req)
	if rec.Code != 200 {
		t.Fatalf("expected 200 got %d", rec.Code)
	}
}
