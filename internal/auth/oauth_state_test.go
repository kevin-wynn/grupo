package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOAuthStateIssueVerify(t *testing.T) {
	o := NewOAuthState("test-secret", false)
	rr := httptest.NewRecorder()
	state, err := o.Issue(rr)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/auth/github/callback", nil)
	for _, c := range rr.Result().Cookies() {
		req.AddCookie(c)
	}
	if !o.Verify(req, state) {
		t.Fatal("expected valid state")
	}
	if o.Verify(req, "wrong") {
		t.Fatal("expected invalid state")
	}
}

func TestOAuthStateExpired(t *testing.T) {
	o := NewOAuthState("test-secret", false)
	expired := time.Now().UTC().Add(-time.Minute).Unix()
	payload := fmt.Sprintf("state123.%d", expired)
	sig := sign("test-secret", payload)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: oauthStateCookie, Value: payload + "." + sig})
	if o.Verify(req, "state123") {
		t.Fatal("expected expired state to fail")
	}
}
