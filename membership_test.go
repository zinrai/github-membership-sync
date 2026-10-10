package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "members.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRefusesInvalidFiles(t *testing.T) {
	valid := `
members:
  alice: admin
  bob: member
teams:
  platform:
    alice: maintainer
  empty: {}
`
	if _, err := loadMembership(writeFile(t, valid)); err != nil {
		t.Fatalf("a valid file was rejected: %v", err)
	}

	for name, content := range map[string]string{
		"misspelt key": `
members:
  alice: admin
team:
  platform: {}
`,
		"no admin": `
members:
  bob: member
`,
		"team left as null": `
members:
  alice: admin
teams:
  platform:
`,
		"logins differing only in case": `
members:
  alice: admin
  Alice: member
`,
		"team member outside the organization": `
members:
  alice: admin
teams:
  platform:
    carol: member
`,
	} {
		if _, err := loadMembership(writeFile(t, content)); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}

// The logins include ones YAML reads as booleans, null or numbers when bare.
func TestDumpLoadsBackAsTheSameMembership(t *testing.T) {
	logins := []string{"Alice", "no", "yes", "on", "off", "true", "null", "y", "123", "0x1f", "1e3"}
	m := &membership{
		Members: map[string]string{},
		Teams:   map[string]map[string]string{"platform": {}, "empty": {}},
	}
	for i, login := range logins {
		role := "member"
		if i == 0 {
			role = "admin"
		}
		m.Members[login] = role
		m.Teams["platform"][login] = "maintainer"
	}

	out, err := encodeMembership(m)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := loadMembership(writeFile(t, string(out)))
	if err != nil {
		t.Fatalf("dump output was rejected: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(loaded, m) {
		t.Errorf("dump output loaded as\n%#v\nwant\n%#v\n%s", loaded, m, out)
	}
	if !strings.Contains(string(out), "empty: {}") {
		t.Errorf("empty team not written as {}:\n%s", out)
	}
}
