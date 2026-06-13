package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/kevin-wynn/grupo/internal/db"
)

const SessionCookie = "grupo_session"

type Manager struct {
	store  *db.Store
	secure bool
}

func NewManager(store *db.Store, secure bool) *Manager {
	return &Manager{store: store, secure: secure}
}

func (m *Manager) CreateSession(ctx context.Context, userID int64, login string) (string, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	sess := db.Session{
		ID:           token,
		GitHubUserID: userID,
		GitHubLogin:  login,
		ExpiresAt:    time.Now().UTC().Add(7 * 24 * time.Hour),
	}
	if err := m.store.CreateSession(ctx, sess); err != nil {
		return "", err
	}
	return token, nil
}

func (m *Manager) SetCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   7 * 24 * 3600,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   m.secure,
		MaxAge:   -1,
	})
}

func (m *Manager) UserFromRequest(ctx context.Context, r *http.Request) (*db.Session, error) {
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil, err
	}
	sess, err := m.store.GetSession(ctx, cookie.Value)
	if err != nil {
		return nil, err
	}
	if time.Now().UTC().After(sess.ExpiresAt) {
		_ = m.store.DeleteSession(ctx, sess.ID)
		return nil, http.ErrNoCookie
	}
	return sess, nil
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
