// Placeholder handlers for Task 15's command registry and Task 16's
// (*Bot).handle callback routing.
//
// Task 17 replaces this file with real search results (tappable, no raw IDs
// ever shown), a /recent listing, and callback routing for the Retry and
// pagination buttons. Nothing here should be extended — build that out in
// Task 17 instead of here, to avoid duplicating work and creating merge
// friction.
package telegram

import "context"

// handleHelp is genuinely complete: it renders /help from the same registry
// that defines it, so the two can never disagree. Task 17 does not need to
// touch this one.
func handleHelp(bot *Bot, update Update, args string) error {
	_, err := bot.Client.SendMessage(context.Background(), bot.ChatID, HelpText(Commands), nil)
	return err
}

// handleSearch is a stub. Task 17 replaces this with real search: matching
// ideas rendered as tappable results, never raw IDs.
func handleSearch(bot *Bot, update Update, args string) error {
	_, err := bot.Client.SendMessage(context.Background(), bot.ChatID, "Search isn't implemented yet.", nil)
	return err
}

// handleRecent is a stub. Task 17 replaces this with the ten newest ideas
// rendered as tappable results.
func handleRecent(bot *Bot, update Update, args string) error {
	_, err := bot.Client.SendMessage(context.Background(), bot.ChatID, "Recent isn't implemented yet.", nil)
	return err
}

// handleCallback is a stub that only acknowledges the callback query so
// Telegram stops showing a loading spinner on the button. Task 17 replaces
// this with real routing (retry, pagination, open) keyed off
// update.CallbackQuery.Data.
func (b *Bot) handleCallback(ctx context.Context, update Update) error {
	return b.Client.AnswerCallbackQuery(ctx, update.CallbackQuery.ID, "")
}
