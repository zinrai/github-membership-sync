package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

const (
	opAdd    = "add"
	opChange = "change"
	opRemove = "remove"
)

type change struct {
	op    string
	team  string
	login string
	from  string
	to    string
}

func (c change) String() string {
	scope := "org"
	if c.team != "" {
		scope = "team " + c.team
	}
	switch c.op {
	case opAdd:
		return fmt.Sprintf("%s %s %s %s", c.op, scope, c.login, c.to)
	case opChange:
		return fmt.Sprintf("%s %s %s %s -> %s", c.op, scope, c.login, c.from, c.to)
	default:
		return fmt.Sprintf("%s %s %s", c.op, scope, c.login)
	}
}

func plan(want, have *membership) []change {
	orgSet, orgRemove := compare("", want.Members, have.Members)

	var teamSet, teamRemove []change
	for _, slug := range want.teamSlugs() {
		set, remove := compare(slug, want.Teams[slug], have.Teams[slug])
		teamSet = append(teamSet, set...)
		teamRemove = append(teamRemove, remove...)
	}

	// Members join the organization before any team and leave their teams
	// before the organization, so a run stopped halfway leaves nobody in a
	// team outside it.
	return slices.Concat(orgSet, teamSet, teamRemove, orgRemove)
}

func compare(team string, want, have map[string]string) (set, remove []change) {
	// Compared without regard to case, as GitHub does. Compared as written, a
	// login spelt differently from GitHub would be removed and added back.
	haveByLower := make(map[string]string, len(have))
	for login := range have {
		haveByLower[strings.ToLower(login)] = login
	}
	wantLower := lowerKeys(want)

	for _, login := range sortedLogins(want) {
		current, ok := haveByLower[strings.ToLower(login)]
		switch {
		case !ok:
			set = append(set, change{op: opAdd, team: team, login: login, to: want[login]})
		case have[current] != want[login]:
			set = append(set, change{op: opChange, team: team, login: login, from: have[current], to: want[login]})
		}
	}
	for _, login := range sortedLogins(have) {
		if !wantLower[strings.ToLower(login)] {
			remove = append(remove, change{op: opRemove, team: team, login: login})
		}
	}
	return set, remove
}

func sortedLogins(logins map[string]string) []string {
	return slices.SortedFunc(maps.Keys(logins), func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
}

func countOrgRemovals(changes []change) int {
	n := 0
	for _, c := range changes {
		if c.team == "" && c.op == opRemove {
			n++
		}
	}
	return n
}
