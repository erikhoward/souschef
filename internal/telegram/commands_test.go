package telegram

import (
	"regexp"
	"strings"
	"testing"
)

// Telegram's own constraints. A malformed entry must fail at startup rather
// than at the setMyCommands call.
var telegramName = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

func TestRegistryMeetsTelegramConstraints(t *testing.T) {
	if err := ValidateRegistry(Commands); err != nil {
		t.Fatalf("the shipped registry must be valid: %v", err)
	}

	for _, c := range Commands {
		if !telegramName.MatchString(c.Name) {
			t.Errorf("name %q must be 1-32 chars of [a-z0-9_]", c.Name)
		}
		if n := len(c.Desc); n < 3 || n > 256 {
			t.Errorf("description for /%s is %d chars, must be 3-256", c.Name, n)
		}
	}
}

// The invariant the design is built on: a command in the menu with no handler
// must be impossible. Handler is a func field with no usable zero value, so
// this is really a belt-and-braces check on the shipped slice.
func TestEveryCommandHasAHandler(t *testing.T) {
	for _, c := range Commands {
		if c.Handler == nil {
			t.Errorf("/%s appears in the registry with no handler", c.Name)
		}
	}
}

func TestValidateRegistryRejectsBadEntries(t *testing.T) {
	cases := map[string][]Command{
		"uppercase name":   {{Name: "Search", Desc: "Search ideas", Handler: noopHandler}},
		"name with space":  {{Name: "find idea", Desc: "Search ideas", Handler: noopHandler}},
		"empty name":       {{Name: "", Desc: "Search ideas", Handler: noopHandler}},
		"short desc":       {{Name: "s", Desc: "hi", Handler: noopHandler}},
		"missing handler":  {{Name: "s", Desc: "Search ideas"}},
		"duplicate name": {
			{Name: "s", Desc: "Search ideas", Handler: noopHandler},
			{Name: "s", Desc: "Search again", Handler: noopHandler},
		},
	}

	for label, registry := range cases {
		if err := ValidateRegistry(registry); err == nil {
			t.Errorf("%s should have been rejected", label)
		}
	}
}

func TestLookupFindsRegisteredCommands(t *testing.T) {
	for _, c := range Commands {
		if _, ok := Lookup(c.Name); !ok {
			t.Errorf("Lookup(%q) failed for a registered command", c.Name)
		}
	}
	if _, ok := Lookup("definitely_not_a_command"); ok {
		t.Error("Lookup must not invent commands")
	}
}

// The menu published to Telegram is derived from the registry, never written
// by hand — that is what makes drift impossible.
func TestMenuIsDerivedFromRegistry(t *testing.T) {
	menu := MenuFrom(Commands)

	if len(menu) != len(Commands) {
		t.Fatalf("menu has %d entries, registry has %d", len(menu), len(Commands))
	}
	for i, c := range Commands {
		if menu[i].Command != c.Name {
			t.Errorf("menu[%d].Command = %q, want %q", i, menu[i].Command, c.Name)
		}
		if menu[i].Description != c.Desc {
			t.Errorf("menu[%d].Description = %q, want %q", i, menu[i].Description, c.Desc)
		}
		// Args is documentation for /help only — Telegram has no parameter
		// syntax, and including it would show literal "<query>" in the menu.
		if strings.Contains(menu[i].Command, "<") {
			t.Errorf("menu[%d].Command leaked Args: %q", i, menu[i].Command)
		}
	}
}

// /help must cover exactly the registry: no missing entries, no extras.
func TestHelpTextCoversExactlyTheRegistry(t *testing.T) {
	help := HelpText(Commands)

	for _, c := range Commands {
		if !strings.Contains(help, "/"+c.Name) {
			t.Errorf("/help omits /%s", c.Name)
		}
		if !strings.Contains(help, c.Desc) {
			t.Errorf("/help omits the description for /%s", c.Name)
		}
		if c.Args != "" && !strings.Contains(help, c.Args) {
			t.Errorf("/help omits the argument hint %q for /%s", c.Args, c.Name)
		}
	}

	// Count slash-prefixed lines to catch extras that are not in the registry.
	lines := 0
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "/") {
			lines++
		}
	}
	if lines != len(Commands) {
		t.Errorf("/help lists %d commands, registry has %d", lines, len(Commands))
	}
}

func TestMenusEqualIgnoresOrder(t *testing.T) {
	a := []BotCommand{{Command: "s", Description: "Search"}, {Command: "help", Description: "Help"}}
	b := []BotCommand{{Command: "help", Description: "Help"}, {Command: "s", Description: "Search"}}

	if !menusEqual(a, b) {
		t.Error("menus differing only in order must compare equal — otherwise every restart writes to the API")
	}
	if menusEqual(a, []BotCommand{{Command: "s", Description: "Search ideas"}}) {
		t.Error("a genuine difference must be detected")
	}
}

func noopHandler(*Bot, Update, string) error { return nil }
