package cmd

import (
	"strings"
	"testing"
)

func TestReferenceCoversAllResourceCommands(t *testing.T) {
	root := newRootCmd()

	body := ""
	for _, c := range root.Commands() {
		if c.Name() == "reference" {
			body = c.Long
			break
		}
	}
	if body == "" {
		t.Fatal("reference command not found in root tree")
	}

	// Every resource command must appear as a section header in the reference.
	want := []string{
		"maxx-cli provider",
		"maxx-cli token",
		"maxx-cli route",
		"maxx-cli strategy",
		"maxx-cli user",
		"maxx-cli invite",
		"maxx-cli settings",
		"maxx-cli login",
		"maxx-cli logout",
		"maxx-cli context",
		// And a representative leaf with a flag, so we know walking goes deep.
		"maxx-cli route set-weight",
		"maxx-cli strategy sticky",
	}
	for _, w := range want {
		if !strings.Contains(body, "## "+w) && !strings.Contains(body, "### "+w) {
			t.Errorf("reference missing section for %q", w)
		}
	}

	// Topic commands themselves must NOT appear (they describe themselves).
	for _, skip := range []string{"## maxx-cli reference", "## maxx-cli formatting", "## maxx-cli auth-config"} {
		if strings.Contains(body, skip) {
			t.Errorf("reference unexpectedly includes topic section %q", skip)
		}
	}
}

func TestHelpTopicsArePresent(t *testing.T) {
	root := newRootCmd()
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"reference", "formatting", "auth-config"} {
		if !names[want] {
			t.Errorf("help topic %q missing from root tree", want)
		}
	}
}

func TestResourceCommandsAreGrouped(t *testing.T) {
	root := newRootCmd()
	expected := map[string]string{
		"login":    groupAuth,
		"logout":   groupAuth,
		"context":  groupAuth,
		"provider": groupResources,
		"token":    groupResources,
		"route":    groupResources,
		"strategy": groupResources,
		"user":     groupResources,
		"invite":   groupResources,
		"settings": groupResources,
	}
	for _, c := range root.Commands() {
		want, ok := expected[c.Name()]
		if !ok {
			continue
		}
		if c.GroupID != want {
			t.Errorf("%s.GroupID = %q, want %q", c.Name(), c.GroupID, want)
		}
	}
}

// TestEveryRootSubcommandHasAGroup catches the "forgot to set GroupID"
// regression — a new command without a group falls into the catch-all
// "Additional Commands" section in --help, which is exactly what the
// gh-style layout is meant to avoid.
func TestEveryRootSubcommandHasAGroup(t *testing.T) {
	root := newRootCmd()
	// Cobra registers `help` and `completion` itself, with no GroupID. They
	// are the only legitimate ungrouped commands.
	exempt := map[string]bool{"help": true, "completion": true}

	allowed := map[string]bool{
		groupAuth:      true,
		groupResources: true,
		groupTopics:    true,
	}

	for _, c := range root.Commands() {
		if exempt[c.Name()] {
			continue
		}
		if c.GroupID == "" {
			t.Errorf("%q has no GroupID — it will land in the catch-all 'Additional Commands' section in --help", c.Name())
			continue
		}
		if !allowed[c.GroupID] {
			t.Errorf("%q has unknown GroupID %q (allowed: %v)", c.Name(), c.GroupID, allowed)
		}
	}
}
