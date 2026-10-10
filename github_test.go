package main

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func newTestClient(t *testing.T, handler http.Handler) *client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	tokenFile := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(tokenFile, []byte("ghs_x\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return newClient(srv.URL, tokenFile)
}

// pages answers with pages[0] when no cursor is given, and pages[n] for "n".
func pages(pages ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct{ Variables struct{ After string } }
		json.NewDecoder(r.Body).Decode(&req)
		i, _ := strconv.Atoi(req.Variables.After)
		io.WriteString(w, pages[i])
	}
}

func TestOrgMembersReadsEveryPage(t *testing.T) {
	gh := newTestClient(t, pages(
		`{"data":{"organization":{"membersWithRole":{"pageInfo":{"hasNextPage":true,"endCursor":"1"},"edges":[{"role":"ADMIN","node":{"login":"alice"}}]}}}}`,
		`{"data":{"organization":{"membersWithRole":{"pageInfo":{"hasNextPage":false},"edges":[{"role":"MEMBER","node":{"login":"bob"}}]}}}}`,
	))
	got, err := gh.orgMembers(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"alice": "admin", "bob": "member"}; !maps.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
}

func TestTeamMembersReadsEveryPage(t *testing.T) {
	gh := newTestClient(t, pages(
		`{"data":{"organization":{"team":{"members":{"pageInfo":{"hasNextPage":true,"endCursor":"1"},"edges":[{"role":"MAINTAINER","node":{"login":"alice"}}]}}}}}`,
		`{"data":{"organization":{"team":{"members":{"pageInfo":{"hasNextPage":false},"edges":[{"role":"MEMBER","node":{"login":"bob"}}]}}}}}`,
	))
	got, err := gh.teamMembers(context.Background(), "example", "platform")
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]string{"alice": "maintainer", "bob": "member"}; !maps.Equal(got, want) {
		t.Errorf("members = %v, want %v", got, want)
	}
}

func TestTeamSlugsReadsEveryPage(t *testing.T) {
	gh := newTestClient(t, pages(
		`{"data":{"organization":{"teams":{"pageInfo":{"hasNextPage":true,"endCursor":"1"},"nodes":[{"slug":"a"}]}}}}`,
		`{"data":{"organization":{"teams":{"pageInfo":{"hasNextPage":false},"nodes":[{"slug":"b"}]}}}}`,
	))
	got, err := gh.teamSlugs(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b"}; !slices.Equal(got, want) {
		t.Errorf("slugs = %q, want %q", got, want)
	}
}

func TestTeamMembersAsksForDirectMembersOnly(t *testing.T) {
	gh := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "membership: IMMEDIATE") {
			http.Error(w, "query includes members of child teams", http.StatusBadRequest)
			return
		}
		io.WriteString(w, `{"data":{"organization":{"team":{"members":{"pageInfo":{"hasNextPage":false},"edges":[]}}}}}`)
	}))
	if _, err := gh.teamMembers(context.Background(), "example", "platform"); err != nil {
		t.Error(err)
	}
}

func TestGraphQLErrorsFailEvenWithData(t *testing.T) {
	gh := newTestClient(t, pages(
		`{"data":{"organization":{"membersWithRole":{"pageInfo":{"hasNextPage":false},"edges":[]}}},"errors":[{"message":"Resource not accessible by integration"}]}`,
	))
	if _, err := gh.orgMembers(context.Background(), "example"); err == nil {
		t.Error("a response with errors was taken as data")
	}
}
