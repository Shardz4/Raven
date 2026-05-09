package bots

import (
	"fmt"
	"log"
	"strings"
	"sync"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// StartTelegram initializes and runs the Telegram bot using long-polling.
// This function blocks until the bot is stopped; call it in a goroutine.
func StartTelegram(token string, svc *BotService) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Printf("❌ [telegram] Failed to start: %v", err)
		return
	}

	log.Printf("✓ [telegram] Authorized as @%s", bot.Self.UserName)

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message == nil {
			continue
		}

		go handleTelegramMessage(bot, update.Message, svc)
	}
}

// handleTelegramMessage routes a Telegram message to the appropriate command handler.
func handleTelegramMessage(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, svc *BotService) {
	if !msg.IsCommand() {
		return
	}

	switch msg.Command() {
	case "start":
		handleTelegramStart(bot, msg)
	case "help":
		handleTelegramHelp(bot, msg)
	case "solve":
		handleTelegramSolve(bot, msg, svc)
	case "status":
		handleTelegramStatus(bot, msg, svc)
	case "leaderboard":
		handleTelegramLeaderboard(bot, msg, svc)
	default:
		sendTelegram(bot, msg.Chat.ID, "❓ Unknown command. Use /help to see available commands.")
	}
}

// ── Command Handlers ──

func handleTelegramStart(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	text := `🪶 *Welcome to Raven — Autonomous AI Developer*

I resolve GitHub issues on autopilot using multiple AI models, Docker sandbox testing, and the RavenMind consensus engine.

*Available Commands:*
/solve <issue\_url> — Submit a GitHub issue for resolution
/status <job\_id> — Check job status
/leaderboard — View model win-rate rankings
/help — Show this message

_Send /solve with a GitHub issue URL to get started!_`

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = tgbotapi.ModeMarkdown
	reply.ReplyToMessageID = msg.MessageID
	if _, err := bot.Send(reply); err != nil {
		log.Printf("[telegram] send error: %v", err)
	}
}

func handleTelegramHelp(bot *tgbotapi.BotAPI, msg *tgbotapi.Message) {
	text := `🪶 *Raven Commands*

/solve <issue\_url> — Submit a GitHub issue for AI resolution
/status <job\_id> — Check the status of a submitted job
/leaderboard — View model win-rate rankings
/help — Show this help message

*Example:*
` + "`/solve https://github.com/owner/repo/issues/42`"

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = tgbotapi.ModeMarkdown
	if _, err := bot.Send(reply); err != nil {
		log.Printf("[telegram] send error: %v", err)
	}
}

func handleTelegramSolve(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, svc *BotService) {
	issueURL := strings.TrimSpace(msg.CommandArguments())
	if issueURL == "" {
		sendTelegram(bot, msg.Chat.ID, "⚠️ Usage: `/solve https://github.com/owner/repo/issues/42`")
		return
	}

	// Send an initial "processing" message that we'll edit with progress updates
	initMsg := tgbotapi.NewMessage(msg.Chat.ID, "⏳ Submitting job...")
	initMsg.ReplyToMessageID = msg.MessageID
	sent, err := bot.Send(initMsg)
	if err != nil {
		log.Printf("[telegram] send error: %v", err)
		return
	}

	// Collect progress events and periodically update the message
	var mu sync.Mutex
	var lastLines []string
	const maxLines = 15

	onEvent := func(event string) {
		if event == "[DONE]" {
			return
		}
		mu.Lock()
		lastLines = append(lastLines, event)
		// Keep only the last N lines to avoid hitting Telegram message limits
		if len(lastLines) > maxLines {
			lastLines = lastLines[len(lastLines)-maxLines:]
		}
		text := "🔄 *Processing...*\n\n```\n" + strings.Join(lastLines, "\n") + "\n```"
		mu.Unlock()

		edit := tgbotapi.NewEditMessageText(msg.Chat.ID, sent.MessageID, text)
		edit.ParseMode = tgbotapi.ModeMarkdown
		if _, err := bot.Send(edit); err != nil {
			// Telegram may reject edits if content is unchanged; ignore
			log.Printf("[telegram] edit error (non-fatal): %v", err)
		}
	}

	jobID, err := svc.SolveIssue(issueURL, onEvent)
	if err != nil {
		edit := tgbotapi.NewEditMessageText(msg.Chat.ID, sent.MessageID,
			fmt.Sprintf("❌ Failed to submit: %s", err.Error()))
		bot.Send(edit)
		return
	}

	// Update the message with the job ID
	statusText := fmt.Sprintf("✅ Job `%s` submitted!\n\n"+
		"Track progress with: `/status %s`\n"+
		"Live updates will appear above as the AI models work.", jobID, jobID)
	edit := tgbotapi.NewEditMessageText(msg.Chat.ID, sent.MessageID, statusText)
	edit.ParseMode = tgbotapi.ModeMarkdown
	bot.Send(edit)
}

func handleTelegramStatus(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, svc *BotService) {
	jobID := strings.TrimSpace(msg.CommandArguments())
	if jobID == "" {
		sendTelegram(bot, msg.Chat.ID, "⚠️ Usage: `/status <job_id>`")
		return
	}

	job, err := svc.GetJobStatus(jobID)
	if err != nil {
		sendTelegram(bot, msg.Chat.ID, fmt.Sprintf("❌ Job not found: `%s`", jobID))
		return
	}

	text := FormatJobStatus(job)

	// If completed, also show a snippet of the winning code
	if job.Status == "completed" && job.WinnerCode != "" {
		codeSnippet := job.WinnerCode
		if len(codeSnippet) > 1000 {
			codeSnippet = codeSnippet[:1000] + "\n... (truncated)"
		}
		text += fmt.Sprintf("\n💻 *Winning Patch:*\n```\n%s\n```", codeSnippet)
	}

	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = tgbotapi.ModeMarkdown
	reply.ReplyToMessageID = msg.MessageID
	if _, err := bot.Send(reply); err != nil {
		log.Printf("[telegram] send error: %v", err)
	}
}

func handleTelegramLeaderboard(bot *tgbotapi.BotAPI, msg *tgbotapi.Message, svc *BotService) {
	entries, err := svc.GetLeaderboard()
	if err != nil {
		sendTelegram(bot, msg.Chat.ID, "❌ Failed to fetch leaderboard.")
		return
	}

	text := FormatLeaderboard(entries)
	reply := tgbotapi.NewMessage(msg.Chat.ID, text)
	reply.ParseMode = tgbotapi.ModeMarkdown
	if _, err := bot.Send(reply); err != nil {
		log.Printf("[telegram] send error: %v", err)
	}
}

// ── Helpers ──

func sendTelegram(bot *tgbotapi.BotAPI, chatID int64, text string) {
	reply := tgbotapi.NewMessage(chatID, text)
	reply.ParseMode = tgbotapi.ModeMarkdown
	if _, err := bot.Send(reply); err != nil {
		log.Printf("[telegram] send error: %v", err)
	}
}
