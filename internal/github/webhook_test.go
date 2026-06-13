package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestVerifyWebhookSignature(t *testing.T) {
	secret := "test-secret"
	body := []byte(`{"ref":"refs/heads/main"}`)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if !VerifyWebhookSignature(secret, body, sig) {
		t.Fatal("expected valid signature")
	}
	if VerifyWebhookSignature(secret, body, "sha256=deadbeef") {
		t.Fatal("expected invalid signature")
	}
}

func TestBranchFromRef(t *testing.T) {
	if got := BranchFromRef("refs/heads/main"); got != "main" {
		t.Fatalf("got %q", got)
	}
}
