package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

// fakeGitHub answers every GraphQL query with a single page and records every
// other request.
type fakeGitHub struct {
	members map[string]string            // login to GraphQL role
	teams   map[string]map[string]string // slug to login to GraphQL role

	mu     sync.Mutex
	writes []string
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/graphql" {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.writes = append(f.writes, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+string(body)))
		f.mu.Unlock()
		return
	}
	var req struct {
		Query     string
		Variables struct{ Slug string }
	}
	json.NewDecoder(r.Body).Decode(&req)

	org := map[string]any{"membersWithRole": connection(f.members)}
	if strings.Contains(req.Query, "team(slug") {
		org = map[string]any{"team": map[string]any{"members": connection(f.teams[req.Variables.Slug])}}
	}
	json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"organization": org}})
}

func connection(members map[string]string) map[string]any {
	edges := []any{}
	for login, role := range members {
		edges = append(edges, map[string]any{"role": role, "node": map[string]any{"login": login}})
	}
	return map[string]any{"pageInfo": map[string]any{"hasNextPage": false}, "edges": edges}
}

func (f *fakeGitHub) writesSoFar() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.writes
}

const fileWithChanges = `
members:
  alice: admin
  bob: member
  carol: member
teams:
  platform:
    alice: maintainer
    carol: member
`

func startSync(t *testing.T, f *fakeGitHub, command string) (*client, *config) {
	t.Helper()
	gh := newTestClient(t, f)
	cfg := &config{command: command, org: "example", configPath: writeFile(t, fileWithChanges), maxRemovals: 3}
	return gh, cfg
}

func TestDiffPrintsThePlanToStdoutAndChangesNothing(t *testing.T) {
	f := &fakeGitHub{
		members: map[string]string{"alice": "ADMIN", "bob": "MEMBER", "dave": "MEMBER"},
		teams:   map[string]map[string]string{"platform": {"alice": "MAINTAINER"}},
	}
	gh, cfg := startSync(t, f, cmdDiff)

	var stdout bytes.Buffer
	if err := runCommand(context.Background(), gh, cfg, &stdout); err != nil {
		t.Fatal(err)
	}
	want := "+ org carol member\n+ team platform carol member\n- org dave\n"
	if stdout.String() != want {
		t.Errorf("stdout =\n%s\nwant\n%s", stdout.String(), want)
	}
	if w := f.writesSoFar(); len(w) != 0 {
		t.Errorf("diff made changes: %q", w)
	}
}

func TestApplyPastMaxRemovalsChangesNothing(t *testing.T) {
	f := &fakeGitHub{
		members: map[string]string{"alice": "ADMIN", "bob": "MEMBER", "carol": "MEMBER"},
		teams:   map[string]map[string]string{"platform": {"alice": "MAINTAINER", "carol": "MEMBER"}},
	}
	for i := range 4 {
		f.members[fmt.Sprintf("leaver%d", i)] = "MEMBER"
	}
	gh, cfg := startSync(t, f, cmdApply)

	if err := runCommand(context.Background(), gh, cfg, io.Discard); err == nil {
		t.Error("a plan removing 4 members ran with -max-removals 3")
	}
	if w := f.writesSoFar(); len(w) != 0 {
		t.Errorf("made changes past -max-removals: %q", w)
	}
}
