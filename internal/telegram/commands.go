package telegram

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// HandlerFunc handles one command. args is everything after the command word.
type HandlerFunc func(bot *Bot, update Update, args string) error

// Command binds a command's name, its user-facing description, and its
// handler in one place.
type Command struct {
	Name    string // as typed, without the leading slash
	Args    string // documentation for /help only; Telegram has no parameter syntax
	Desc    string // shown in Telegram's command menu
	Handler HandlerFunc
}

// Commands is the single source of truth for what this bot can do.
//
// Three consumers read this slice and nothing else: the menu published via
// setMyCommands, the update router, and /help. BotFather is never used to
// configure commands — it only issues the token.
//
// Because Handler has no usable zero value, a menu entry with no handler
// cannot be written without failing validation at startup.
//
// Populated from init(), not a var initializer: the handlers read Commands
// (for /help and search-result rendering), and a var initializer that both
// stores a function value and is read by that function's body is an
// initialization cycle as far as the compiler's dependency analysis is
// concerned, even though nothing is actually invoked during initialization.
// init() runs after all package-level variables have their zero/initializer
// values, which breaks the cycle.
var Commands []Command

func init() {
	Commands = []Command{
		{
			Name:    "s",
			Args:    "<query>",
			Desc:    "Search your ideas",
			Handler: handleSearch,
		},
		{
			Name:    "recent",
			Desc:    "Show your ten newest ideas",
			Handler: handleRecent,
		},
		{
			Name:    "help",
			Desc:    "What this bot can do",
			Handler: handleHelp,
		},
	}
}

var namePattern = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// ValidateRegistry checks the registry against Telegram's constraints so a
// malformed entry fails at startup rather than at the API call.
func ValidateRegistry(registry []Command) error {
	seen := map[string]bool{}

	for _, c := range registry {
		if !namePattern.MatchString(c.Name) {
			return fmt.Errorf("command %q: name must be 1-32 characters of lowercase letters, digits, and underscores", c.Name)
		}
		if seen[c.Name] {
			return fmt.Errorf("command %q is registered twice", c.Name)
		}
		seen[c.Name] = true

		if n := len(c.Desc); n < 3 || n > 256 {
			return fmt.Errorf("command %q: description is %d characters, must be 3-256", c.Name, n)
		}
		if c.Handler == nil {
			return fmt.Errorf("command %q has no handler", c.Name)
		}
	}
	return nil
}

func Lookup(name string) (Command, bool) {
	for _, c := range Commands {
		if c.Name == name {
			return c, true
		}
	}
	return Command{}, false
}

// MenuFrom derives the Telegram menu. Args is deliberately excluded — it is
// documentation for /help, and Telegram would render it literally.
func MenuFrom(registry []Command) []BotCommand {
	out := make([]BotCommand, 0, len(registry))
	for _, c := range registry {
		out = append(out, BotCommand{Command: c.Name, Description: c.Desc})
	}
	return out
}

// HelpText renders /help from the same slice, so the two can never disagree.
func HelpText(registry []Command) string {
	var b strings.Builder
	b.WriteString("Send me any message to capture it as an idea. A voice note works too.\n\n")

	for _, c := range registry {
		b.WriteString("/" + c.Name)
		if c.Args != "" {
			b.WriteString(" " + c.Args)
		}
		b.WriteString(" — " + c.Desc + "\n")
	}
	return b.String()
}

// menusEqual compares order-independently, so a restart that changed nothing
// does not write to a rate-limited API.
func menusEqual(a, b []BotCommand) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(list []BotCommand) []string {
		out := make([]string, 0, len(list))
		for _, c := range list {
			out = append(out, c.Command+"\x00"+c.Description)
		}
		sort.Strings(out)
		return out
	}
	left, right := key(a), key(b)
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

// SyncCommands publishes the menu only when it differs from what Telegram
// already has, turning a real change into one log line and a no-op restart
// into nothing at all.
func SyncCommands(ctx context.Context, client *Client, chatID int64, registry []Command) (bool, error) {
	if err := ValidateRegistry(registry); err != nil {
		return false, err
	}

	want := MenuFrom(registry)

	current, err := client.GetMyCommands(ctx, chatID)
	if err != nil {
		return false, fmt.Errorf("read current command menu: %w", err)
	}
	if menusEqual(current, want) {
		return false, nil
	}
	if err := client.SetMyCommands(ctx, chatID, want); err != nil {
		return false, fmt.Errorf("publish command menu: %w", err)
	}
	return true, nil
}
