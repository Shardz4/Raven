package bots

import (
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"
)

// Discord embed colors
const (
	colorBlue   = 0x58A6FF // Raven brand blue
	colorGreen  = 0x238636 // Success
	colorRed    = 0xDA3633 // Error
	colorYellow = 0xD29922 // Warning / in progress
)

// StartDiscord initializes and runs the Discord bot.
// This function blocks until the session is closed; call it in a goroutine.
func StartDiscord(token string, svc *BotService) {
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Printf("❌ [discord] Failed to create session: %v", err)
		return
	}

	dg.Identify.Intents = discordgo.IntentsGuildMessages | discordgo.IntentsDirectMessages

	// Register slash commands on ready
	dg.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Printf("✓ [discord] Logged in as %s#%s", r.User.Username, r.User.Discriminator)
		registerDiscordCommands(s)
	})

	// Handle slash command interactions
	dg.AddHandler(func(s *discordgo.Session, i *discordgo.InteractionCreate) {
		handleDiscordInteraction(s, i, svc)
	})

	if err := dg.Open(); err != nil {
		log.Printf("❌ [discord] Failed to open connection: %v", err)
		return
	}

	log.Println("✓ [discord] Bot is running")

	// Block forever (the caller should run this in a goroutine)
	select {}
}

// registerDiscordCommands registers slash commands with Discord.
func registerDiscordCommands(s *discordgo.Session) {
	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "solve",
			Description: "Submit a GitHub issue for AI-powered resolution",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "issue_url",
					Description: "GitHub issue URL (e.g., https://github.com/owner/repo/issues/42)",
					Required:    true,
				},
			},
		},
		{
			Name:        "status",
			Description: "Check the status of a submitted job",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionString,
					Name:        "job_id",
					Description: "The job ID returned when you submitted the issue",
					Required:    true,
				},
			},
		},
		{
			Name:        "leaderboard",
			Description: "View model win-rate rankings",
		},
		{
			Name:        "help",
			Description: "Show available Raven commands",
		},
	}

	for _, cmd := range commands {
		_, err := s.ApplicationCommandCreate(s.State.User.ID, "", cmd)
		if err != nil {
			log.Printf("[discord] Failed to register command /%s: %v", cmd.Name, err)
		}
	}
	log.Printf("[discord] Registered %d slash commands", len(commands))
}

// handleDiscordInteraction routes slash command interactions to handlers.
func handleDiscordInteraction(s *discordgo.Session, i *discordgo.InteractionCreate, svc *BotService) {
	if i.Type != discordgo.InteractionApplicationCommand {
		return
	}

	data := i.ApplicationCommandData()

	switch data.Name {
	case "solve":
		handleDiscordSolve(s, i, svc, data.Options)
	case "status":
		handleDiscordStatus(s, i, svc, data.Options)
	case "leaderboard":
		handleDiscordLeaderboard(s, i, svc)
	case "help":
		handleDiscordHelp(s, i)
	}
}

// ── Command Handlers ──

func handleDiscordHelp(s *discordgo.Session, i *discordgo.InteractionCreate) {
	embed := &discordgo.MessageEmbed{
		Title:       "🪶 Raven — Autonomous AI Developer",
		Description: "I resolve GitHub issues on autopilot using multiple AI models, Docker sandbox testing, and the RavenMind consensus engine.",
		Color:       colorBlue,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "/solve `<issue_url>`", Value: "Submit a GitHub issue for AI resolution", Inline: false},
			{Name: "/status `<job_id>`", Value: "Check the status of a submitted job", Inline: false},
			{Name: "/leaderboard", Value: "View model win-rate rankings", Inline: false},
			{Name: "/help", Value: "Show this help message", Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Raven AI • Powered by RavenMind Consensus",
		},
	}

	respondEmbed(s, i, embed)
}

func handleDiscordSolve(s *discordgo.Session, i *discordgo.InteractionCreate, svc *BotService, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	issueURL := ""
	for _, opt := range opts {
		if opt.Name == "issue_url" {
			issueURL = opt.StringValue()
		}
	}

	// Acknowledge immediately (Discord requires a response within 3 seconds)
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	})

	// Collect live progress events
	var mu sync.Mutex
	var lastLines []string
	const maxLines = 12

	onEvent := func(event string) {
		if event == "[DONE]" {
			return
		}
		mu.Lock()
		lastLines = append(lastLines, event)
		if len(lastLines) > maxLines {
			lastLines = lastLines[len(lastLines)-maxLines:]
		}
		progressText := strings.Join(lastLines, "\n")
		mu.Unlock()

		embed := &discordgo.MessageEmbed{
			Title:       "🔄 Processing...",
			Description: "```\n" + progressText + "\n```",
			Color:       colorYellow,
		}

		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
	}

	jobID, err := svc.SolveIssue(issueURL, onEvent)
	if err != nil {
		embed := &discordgo.MessageEmbed{
			Title:       "❌ Submission Failed",
			Description: err.Error(),
			Color:       colorRed,
		}
		s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
			Embeds: &[]*discordgo.MessageEmbed{embed},
		})
		return
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Job Submitted",
		Description: fmt.Sprintf("Job ID: `%s`\nIssue: %s", jobID, issueURL),
		Color:       colorGreen,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Track Progress", Value: fmt.Sprintf("Use `/status %s` to check results", jobID), Inline: false},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Live progress updates appear above as AI models work",
		},
	}

	s.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Embeds: &[]*discordgo.MessageEmbed{embed},
	})
}

func handleDiscordStatus(s *discordgo.Session, i *discordgo.InteractionCreate, svc *BotService, opts []*discordgo.ApplicationCommandInteractionDataOption) {
	jobID := ""
	for _, opt := range opts {
		if opt.Name == "job_id" {
			jobID = opt.StringValue()
		}
	}

	job, err := svc.GetJobStatus(jobID)
	if err != nil {
		respondText(s, i, fmt.Sprintf("❌ Job not found: `%s`", jobID))
		return
	}

	color := colorBlue
	title := "📋 Job Status"
	switch job.Status {
	case "completed":
		color = colorGreen
		title = "✅ Job Completed"
	case "failed":
		color = colorRed
		title = "❌ Job Failed"
	case "running":
		color = colorYellow
		title = "🔄 Job Running"
	}

	embed := &discordgo.MessageEmbed{
		Title: title,
		Color: color,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Job ID", Value: fmt.Sprintf("`%s`", job.ID), Inline: true},
			{Name: "Status", Value: job.Status, Inline: true},
			{Name: "Issue", Value: job.IssueURL, Inline: false},
		},
	}

	if job.WinnerModel != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name: "🏆 Winner", Value: fmt.Sprintf("`%s`", job.WinnerModel), Inline: true,
		})
	}
	if job.ErrorMessage != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name: "Error", Value: job.ErrorMessage, Inline: false,
		})
	}
	if job.Status == "completed" && job.WinnerCode != "" {
		codeSnippet := job.WinnerCode
		if len(codeSnippet) > 900 {
			codeSnippet = codeSnippet[:900] + "\n... (truncated)"
		}
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name: "💻 Winning Patch", Value: "```\n" + codeSnippet + "\n```", Inline: false,
		})
	}

	respondEmbed(s, i, embed)
}

func handleDiscordLeaderboard(s *discordgo.Session, i *discordgo.InteractionCreate, svc *BotService) {
	entries, err := svc.GetLeaderboard()
	if err != nil {
		respondText(s, i, "❌ Failed to fetch leaderboard.")
		return
	}

	if len(entries) == 0 {
		respondText(s, i, "📊 No leaderboard data yet. Submit some issues first!")
		return
	}

	var desc strings.Builder
	desc.WriteString("```\n")
	desc.WriteString(fmt.Sprintf("%-4s %-25s %5s %5s %7s %6s\n", "Rank", "Model", "Wins", "Total", "WinRate", "AvgScr"))
	desc.WriteString(strings.Repeat("─", 58) + "\n")
	for idx, e := range entries {
		desc.WriteString(fmt.Sprintf("%-4d %-25s %5d %5d %6.1f%% %6.1f\n",
			idx+1, truncate(e.Model, 25), e.Wins, e.Total, e.WinRate*100, e.AvgScore))
	}
	desc.WriteString("```")

	embed := &discordgo.MessageEmbed{
		Title:       "📊 Raven Model Leaderboard",
		Description: desc.String(),
		Color:       colorBlue,
	}

	respondEmbed(s, i, embed)
}

// ── Helpers ──

func respondEmbed(s *discordgo.Session, i *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
		},
	})
}

func respondText(s *discordgo.Session, i *discordgo.InteractionCreate, text string) {
	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: text,
		},
	})
}
