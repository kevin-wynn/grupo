package github

import "testing"

func TestInstallURL(t *testing.T) {
	u := InstallURL("grupo", "abc")
	if u != "https://github.com/apps/grupo/installations/new?state=abc" {
		t.Fatalf("unexpected url: %s", u)
	}
}
