package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
)

var ErrGitHubSignature = errors.New("invalid github signature")

// VerifyGitHub verifies X-Hub-Signature-256: "sha256=<hex>"
func VerifyGitHub(r *http.Request, secret string, maxBody int64) ([]byte, error) {
	sig := strings.TrimSpace(r.Header.Get("X-Hub-Signature-256"))
	if !strings.HasPrefix(sig, "sha256=") {
		return nil, ErrGitHubSignature
	}
	wantHex := strings.TrimPrefix(sig, "sha256=")

	body, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBody))
	if err != nil {
		return nil, err
	}

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	got := mac.Sum(nil)

	want, err := hex.DecodeString(wantHex)
	if err != nil {
		return nil, ErrGitHubSignature
	}
	if !hmac.Equal(got, want) {
		return nil, ErrGitHubSignature
	}
	return body, nil
}

type InboundGitHubWebhookHandler struct{}

func (h *InboundGitHubWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if secret == "" {
		writeErr(w, 500, "missing_github_secret")
		return
	}

	body, err := VerifyGitHub(r, secret, 1<<20)
	if err != nil {
		writeErr(w, 401, "invalid_signature")
		return
	}

	event := r.Header.Get("X-GitHub-Event")
	delivery := r.Header.Get("X-GitHub-Delivery")

	// 这里只做演示：把 payload 原样解析并返回摘要
	var payload map[string]any
	_ = json.Unmarshal(body, &payload)

	writeJSON(w, 200, map[string]any{
		"ok":       true,
		"event":    event,
		"delivery": delivery,
		"has_body": len(body) > 0,
	})
}
