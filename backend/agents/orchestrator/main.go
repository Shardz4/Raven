package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/Shardz4/raven/broker"
	"github.com/Shardz4/raven/config"
	gh "github.com/Shardz4/raven/github"
	"github.com/Shardz4/raven/store"
	"github.com/nats-io/nats.go"
)

func main() {
	log.Println("🪶 Raven Orchestrator Agent Starting...")

	cfg := config.Load()
	db := store.NewClient(cfg.StoreServiceURL)
	fetcher := gh.NewFetcher(cfg.GitHubToken)

	br, err := broker.New(cfg.NatsURL)
	if err != nil {
		log.Fatalf("❌ NATS connection failed: %v", err)
	}
	defer br.Close()

	// Helper to send progress log events
	emit := func(jobID, msg string) {
		event := broker.EventMsg{
			JobID:   jobID,
			Message: msg,
		}
		data, _ := json.Marshal(event)
		_ = br.Publish(broker.SubjectEventsPrefix+jobID, data)
		log.Printf("[job:%s] %s", jobID, msg)
	}

	// Subscribe to jobs
	_, err = br.QueueSubscribe(broker.SubjectJobs, "orchestrator-group", func(msg *nats.Msg) {
		var req broker.JobRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("❌ Failed to parse job request: %v", err)
			return
		}

		go func() {
			log.Printf("[job:%s] Started processing", req.JobID)
			emit(req.JobID, "🔍 Fetching GitHub issue...")

			job, err := db.GetJob(req.JobID)
			if err != nil {
				log.Printf("[job:%s] Error getting job: %v", req.JobID, err)
				return
			}

			issue, err := fetcher.FetchIssue(req.IssueURL)
			if err != nil {
				job.Status = "failed"
				job.ErrorMessage = fmt.Sprintf("Failed to fetch issue: %v", err)
				_ = db.UpdateJobResult(job)
				emit(req.JobID, "❌ "+job.ErrorMessage)
				emit(req.JobID, "[DONE]")
				return
			}

			job.Status = "running"
			job.IssueTitle = issue.Title
			job.Language = issue.Language
			_ = db.UpdateJobResult(job)

			emit(req.JobID, fmt.Sprintf("📋 Issue: %s", issue.Title))
			emit(req.JobID, fmt.Sprintf("🔤 Detected language: %s", issue.Language))

			// Build prompts and find active solvers
			prompt := issue.Prompt()
			providers := cfg.AvailableProviders()

			// Cap by redundancy config
			selected := providers
			if len(selected) > cfg.Redundancy {
				selected = selected[:cfg.Redundancy]
			}

			emit(req.JobID, fmt.Sprintf("📡 Engaging %d AI models in parallel...", len(selected)))

			// Fan out solver calls by publishing PatchRequests
			for _, provider := range selected {
				model := getModelForProvider(provider, cfg)
				patchReq := broker.PatchRequest{
					JobID:    req.JobID,
					Provider: provider,
					Model:    model,
					Prompt:   prompt,
					Language: issue.Language,
				}
				reqData, _ := json.Marshal(patchReq)
				subject := broker.SubjectSolverPrefix + provider
				if err := br.Publish(subject, reqData); err != nil {
					emit(req.JobID, fmt.Sprintf("❌ Failed to contact solver %s: %v", provider, err))
				} else {
					emit(req.JobID, fmt.Sprintf("⚡ Prompting %s/%s...", provider, model))
				}
			}
		}()
	})

	if err != nil {
		log.Fatalf("❌ Failed to subscribe to jobs: %v", err)
	}

	log.Println("✓ Orchestrator ready and listening for jobs...")

	// Keep alive
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func getModelForProvider(p string, cfg *config.Config) string {
	switch p {
	case "openai":
		return "gpt-4o"
	case "anthropic":
		return "claude-3-5-sonnet-20241022"
	case "deepseek":
		return "deepseek-coder"
	case "grok":
		return "grok-beta"
	default:
		return "ollama-local"
	}
}
