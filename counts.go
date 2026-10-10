package main

import (
	"fmt"
	"log/slog"
)

// logRead and logPlan go to the log rather than stdout, so that stdout keeps
// carrying the plan or the YAML alone.
func logRead(org string, m *membership) {
	slog.Info("read", "org", org,
		"members", len(m.Members), "admins", countRole(m.Members, "admin"), "teams", len(m.Teams))
	for _, slug := range m.teamSlugs() {
		team := m.Teams[slug]
		slog.Info("read", "team", slug,
			"direct_members", len(team), "maintainers", countRole(team, "maintainer"))
	}
}

func logPlan(org string, have, want *membership) {
	slog.Info("plan", "org", org,
		"members", fromTo(len(have.Members), len(want.Members)),
		"admins", fromTo(countRole(have.Members, "admin"), countRole(want.Members, "admin")))
	for _, slug := range want.teamSlugs() {
		h, w := have.Teams[slug], want.Teams[slug]
		slog.Info("plan", "team", slug,
			"direct_members", fromTo(len(h), len(w)),
			"maintainers", fromTo(countRole(h, "maintainer"), countRole(w, "maintainer")))
	}
}

func countRole(logins map[string]string, role string) int {
	n := 0
	for _, r := range logins {
		if r == role {
			n++
		}
	}
	return n
}

func fromTo(from, to int) string {
	return fmt.Sprintf("%d->%d", from, to)
}
