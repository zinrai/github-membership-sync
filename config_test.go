package main

import "testing"

func TestParseArgsRefusesARepeatedFlag(t *testing.T) {
	base := []string{"apply", "-org", "a", "-membership-file", "members.yaml", "-token-file", "token"}
	if _, err := parseArgs(base); err != nil {
		t.Fatalf("flags given once were rejected: %v", err)
	}

	for name, extra := range map[string][]string{
		"-org":          {"-org", "b"},
		"-org=":         {"-org=b"},
		"-max-removals": {"-max-removals", "5", "-max-removals", "50"},
	} {
		if _, err := parseArgs(append(base, extra...)); err == nil {
			t.Errorf("repeated %s was accepted", name)
		}
	}
}
