package router

import (
	"net/http"
	"testing"
)

func TestMatcher(t *testing.T) {
	m := NewMatcher("admin.mygrupo.dev")
	m.SetProjects(map[string]string{
		"app1.mygrupo.dev": "proj-1",
	})

	tests := []struct {
		host string
		kind RouteKind
		id   string
	}{
		{"admin.mygrupo.dev", RouteAdmin, ""},
		{"admin.mygrupo.dev:8080", RouteAdmin, ""},
		{"app1.mygrupo.dev", RouteSite, "proj-1"},
		{"unknown.mygrupo.dev", RouteUnknown, ""},
	}

	for _, tt := range tests {
		r := &http.Request{Host: tt.host}
		got := m.Match(r)
		if got.Kind != tt.kind || got.ProjectID != tt.id {
			t.Errorf("Match(%q) = %+v, want kind=%v id=%q", tt.host, got, tt.kind, tt.id)
		}
	}
}
