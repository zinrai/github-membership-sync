package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
)

type membership struct {
	Members map[string]string            `yaml:"members"`
	Teams   map[string]map[string]string `yaml:"teams"`
}

func loadMembership(path string) (*membership, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m membership
	// Without DisallowUnknownField a misspelt "members" reads as an
	// organization with nobody in it, and the plan removes everyone.
	if err := yaml.UnmarshalWithOptions(raw, &m, yaml.DisallowUnknownField()); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &m, nil
}

func (m *membership) validate() error {
	if err := checkUniqueLogins("members", m.Members); err != nil {
		return err
	}
	if !slices.Contains(slices.Collect(maps.Values(m.Members)), "admin") {
		return errors.New("members: no admin, nobody would be left to manage the organization")
	}
	for _, slug := range m.teamSlugs() {
		team := m.Teams[slug]
		// An empty "slug:" also reads as nil. Requiring {} keeps a team from
		// being emptied by deleting its lines and forgetting the key.
		if team == nil {
			return fmt.Errorf("teams: %s: write {} to empty the team", slug)
		}
		if err := checkUniqueLogins("teams: "+slug, team); err != nil {
			return err
		}
		if err := checkInOrg(slug, team, m.Members); err != nil {
			return err
		}
	}
	return nil
}

func checkUniqueLogins(where string, logins map[string]string) error {
	seen := make(map[string]string, len(logins))
	for login := range logins {
		lower := strings.ToLower(login)
		// GitHub takes these as one user, and whichever came last would
		// silently decide the role.
		if other, ok := seen[lower]; ok {
			return fmt.Errorf("%s: %s and %s are the same user", where, other, login)
		}
		seen[lower] = login
	}
	return nil
}

func checkInOrg(slug string, team, members map[string]string) error {
	inOrg := lowerKeys(members)
	for login := range team {
		// Adding a non-member to a team also brings them into the organization,
		// which members would then disagree with.
		if !inOrg[strings.ToLower(login)] {
			return fmt.Errorf("teams: %s: %s is not in members", slug, login)
		}
	}
	return nil
}

func lowerKeys(logins map[string]string) map[string]bool {
	set := make(map[string]bool, len(logins))
	for login := range logins {
		set[strings.ToLower(login)] = true
	}
	return set
}

func (m *membership) teamSlugs() []string {
	return slices.Sorted(maps.Keys(m.Teams))
}

func encodeMembership(m *membership) ([]byte, error) {
	return yaml.Marshal(m)
}
