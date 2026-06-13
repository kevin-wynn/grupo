package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const oauthStateCookie = "grupo_oauth_state"

type OAuthState struct {
	secret string
	secure bool
}

func NewOAuthState(secret string, secure bool) *OAuthState {
	return &OAuthState{secret: secret, secure: secure}
}

func (o *OAuthState) Issue(w http.ResponseWriter) (string, error) {
	state, err := randomToken(24)
	if err != nil {
		return "", err
	}
	expires := time.Now().UTC().Add(10 * time.Minute).Unix()
	payload := fmt.Sprintf("%s.%d", state, expires)
	sig := sign(o.secret, payload)
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    payload + "." + sig,
		Path:     "/auth",
		HttpOnly: true,
		Secure:   o.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	return state, nil
}

func (o *OAuthState) Verify(r *http.Request, state string) bool {
	cookie, err := r.Cookie(oauthStateCookie)
	if err != nil || state == "" {
		return false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 3 {
		return false
	}
	payload := parts[0] + "." + parts[1]
	if !hmac.Equal([]byte(parts[2]), []byte(sign(o.secret, payload))) {
		return false
	}
	expires, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().UTC().Unix() > expires {
		return false
	}
	return parts[0] == state
}

func (o *OAuthState) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     oauthStateCookie,
		Value:    "",
		Path:     "/auth",
		HttpOnly: true,
		Secure:   o.secure,
		MaxAge:   -1,
	})
}

func sign(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}
