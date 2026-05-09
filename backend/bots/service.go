package bots

import (
	"fmt"
	"strings"

	"github.com/Shardz4/raven/api"
	"github.com/Shardz4/raven/config"
	"github.com/Shardz4/raven/store"
)

// BotService bridges chat bot commands to the Raven backend.
// Both the Telegram and Discord bots delegate to this shared service.
type BotService struct {
	server *api.Server
	cfg    *config.Config
}

// NewService creates a new BotService wired to the given API server and config.
func NewService(server *api.Server, cfg *config.Config) *BotService {
	return &BotService{
		server: server,
		cfg:    cfg,
	}
}

// SolveIssue submits a GitHub issue URL for resolution.
// The onEvent callback receives live progress messages (safe to call from goroutines).
// Returns the job ID assigned to this resolution.
func (bs *BotService) SolveIssue(issueURL string, onEvent func(msg string)) (string, error) {
	// Basic validation
	issueURL = strings.TrimSpace(issueURL)
	if issueURL == "" {
		return "", fmt.Errorf("issue URL is required")
	}
	if !strings.HasPrefix(issueURL, "https://github.com/") {
		return "", fmt.Errorf("invalid GitHub issue URL — must start with https://github.com/")
	}

	jobID, err := bs.server.SubmitAndProcessJob(issueURL, onEvent)
	if err != nil {
		return "", fmt.Errorf("failed to submit job: %w", err)
	}
	return jobID, nil
}

// GetJobStatus retrieves the current status of a job by ID.
func (bs *BotService) GetJobStatus(jobID string) (*store.Job, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, fmt.Errorf("job ID is required")
	}
	return bs.server.GetStore().GetJob(jobID)
}

// GetLeaderboard returns the model win-rate leaderboard.
func (bs *BotService) GetLeaderboard() ([]*store.LeaderboardEntry, error) {
	return bs.server.GetStore().GetLeaderboard()
}

// FormatJobStatus produces a human-readable status summary for a job.
func FormatJobStatus(job *store.Job) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Job: %s\n", job.ID))
	sb.WriteString(fmt.Sprintf("🔗 Issue: %s\n", job.IssueURL))

	switch job.Status {
	case "pending":
		sb.WriteString("⏳ Status: Pending\n")
	case "running":
		sb.WriteString("🔄 Status: Running\n")
	case "completed":
		sb.WriteString("✅ Status: Completed\n")
		sb.WriteString(fmt.Sprintf("🏆 Winner: %s\n", job.WinnerModel))
		if job.IssueTitle != "" {
			sb.WriteString(fmt.Sprintf("📝 Title: %s\n", job.IssueTitle))
		}
	case "failed":
		sb.WriteString("❌ Status: Failed\n")
		if job.ErrorMessage != "" {
			sb.WriteString(fmt.Sprintf("💬 Error: %s\n", job.ErrorMessage))
		}
	default:
		sb.WriteString(fmt.Sprintf("❓ Status: %s\n", job.Status))
	}

	return sb.String()
}

// FormatLeaderboard produces a human-readable leaderboard table.
func FormatLeaderboard(entries []*store.LeaderboardEntry) string {
	if len(entries) == 0 {
		return "📊 No leaderboard data yet. Submit some issues first!"
	}

	var sb strings.Builder
	sb.WriteString("📊 **Raven Model Leaderboard**\n\n")
	sb.WriteString("```\n")
	sb.WriteString(fmt.Sprintf("%-4s %-30s %5s %5s %7s %6s\n", "Rank", "Model", "Wins", "Total", "WinRate", "AvgScr"))
	sb.WriteString(strings.Repeat("─", 64) + "\n")

	for i, e := range entries {
		sb.WriteString(fmt.Sprintf("%-4d %-30s %5d %5d %6.1f%% %6.1f\n",
			i+1, truncate(e.Model, 30), e.Wins, e.Total, e.WinRate*100, e.AvgScore))
	}
	sb.WriteString("```")
	return sb.String()
}

// truncate shortens a string to maxLen, adding "…" if truncated.
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-1] + "…"
}
