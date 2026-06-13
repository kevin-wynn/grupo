package github

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func VerifyWebhookSignature(secret string, body []byte, signatureHeader string) bool {
	if secret == "" || signatureHeader == "" {
		return false
	}
	const prefix = "sha256="
	if !strings.HasPrefix(signatureHeader, prefix) {
		return false
	}
	sigHex := strings.TrimPrefix(signatureHeader, prefix)
	expected, err := hex.DecodeString(sigHex)
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), expected)
}

type PushEvent struct {
	Ref        string `json:"ref"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"head_commit"`
}

func ParsePushEvent(body []byte) (*PushEvent, error) {
	var evt PushEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return nil, err
	}
	return &evt, nil
}

func BranchFromRef(ref string) string {
	const prefix = "refs/heads/"
	if strings.HasPrefix(ref, prefix) {
		return strings.TrimPrefix(ref, prefix)
	}
	return ref
}

func ReadWebhookBody(r *http.Request, maxBytes int64) ([]byte, error) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBytes))
	if err != nil {
		return nil, err
	}
	return body, nil
}

type InstallationEvent struct {
	Action       string `json:"action"`
	Installation struct {
		ID      int64  `json:"id"`
		Account struct {
			Login string `json:"login"`
			Type  string `json:"type"`
		} `json:"account"`
	} `json:"installation"`
}

func ParseInstallationEvent(body []byte) (*InstallationEvent, error) {
	var evt InstallationEvent
	if err := json.Unmarshal(body, &evt); err != nil {
		return nil, err
	}
	return &evt, nil
}

func RepoFullName(owner, repo string) string {
	return fmt.Sprintf("%s/%s", owner, repo)
}

func SplitFullName(full string) (owner, repo string, ok bool) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
