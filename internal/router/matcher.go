package router

import (
	"net"
	"net/http"
	"strings"
)

type RouteKind int

const (
	RouteUnknown RouteKind = iota
	RouteAdmin
	RouteSite
)

type Route struct {
	Kind      RouteKind
	ProjectID string
	Domain    string
}

type Matcher struct {
	adminDomain string
	byDomain    map[string]string
}

func NewMatcher(adminDomain string) *Matcher {
	return &Matcher{
		adminDomain: strings.ToLower(adminDomain),
		byDomain:    map[string]string{},
	}
}

func (m *Matcher) SetProjects(domains map[string]string) {
	m.byDomain = make(map[string]string, len(domains))
	for domain, id := range domains {
		m.byDomain[strings.ToLower(domain)] = id
	}
}

func (m *Matcher) Match(r *http.Request) Route {
	host := hostWithoutPort(r.Host)
	host = strings.ToLower(host)
	if host == m.adminDomain {
		return Route{Kind: RouteAdmin, Domain: host}
	}
	if id, ok := m.byDomain[host]; ok {
		return Route{Kind: RouteSite, ProjectID: id, Domain: host}
	}
	return Route{Kind: RouteUnknown, Domain: host}
}

func hostWithoutPort(hostport string) string {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		return hostport
	}
	return host
}
