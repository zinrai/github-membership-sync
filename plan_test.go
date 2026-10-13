package main

import (
	"slices"
	"testing"
)

func lines(changes []change) []string {
	out := make([]string, len(changes))
	for i, c := range changes {
		out[i] = c.String()
	}
	return out
}

func TestPlanComparesLoginsWithoutCase(t *testing.T) {
	want := &membership{
		Members: map[string]string{"alice": "admin"},
		Teams:   map[string]map[string]string{"platform": {"alice": "member"}},
	}
	have := &membership{
		Members: map[string]string{"Alice": "admin"},
		Teams:   map[string]map[string]string{"platform": {"ALICE": "member"}},
	}
	if got := plan(want, have); len(got) != 0 {
		t.Errorf("plan = %q, want nothing", lines(got))
	}
}

func TestPlanOrdersOrganizationAroundTeams(t *testing.T) {
	want := &membership{
		Members: map[string]string{"alice": "admin", "carol": "member", "bob": "admin"},
		Teams:   map[string]map[string]string{"platform": {"carol": "member", "bob": "maintainer"}},
	}
	have := &membership{
		Members: map[string]string{"alice": "admin", "bob": "member", "dave": "member"},
		Teams:   map[string]map[string]string{"platform": {"bob": "member", "dave": "member"}},
	}
	wantLines := []string{
		"change org bob member -> admin",
		"add org carol member",
		"change team platform bob member -> maintainer",
		"add team platform carol member",
		"remove team platform dave",
		"remove org dave",
	}
	if got := lines(plan(want, have)); !slices.Equal(got, wantLines) {
		t.Errorf("plan =\n%q\nwant\n%q", got, wantLines)
	}
}

func TestPlanLeavesTeamsOutsideTheFileAlone(t *testing.T) {
	want := &membership{
		Members: map[string]string{"alice": "admin"},
		Teams:   map[string]map[string]string{},
	}
	have := &membership{
		Members: map[string]string{"alice": "admin"},
		Teams:   map[string]map[string]string{"synced": {"alice": "member", "bob": "member"}},
	}
	if got := plan(want, have); len(got) != 0 {
		t.Errorf("plan = %q, want nothing", lines(got))
	}
}
