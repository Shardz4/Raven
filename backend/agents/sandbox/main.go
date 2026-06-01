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
	"github.com/Shardz4/raven/sandbox"
	"github.com/Shardz4/raven/store"
	"github.com/nats-io/nats.go"
)

func main() {
	log.Println("🪶 Raven Sandbox Agent Starting...")

	cfg := config.Load()
	db := store.NewClient(cfg.StoreServiceURL)

	sb, err := sandbox.NewManager(cfg.SandboxImage, cfg.DockerTimeout)
	if err != nil {
		log.Fatalf("❌ Docker sandbox manager failed: %v", err)
	}
	defer sb.Close()

	// Try to ensure sandbox image is available
	if err := sb.EnsureImage("../sandbox_env"); err != nil {
		log.Printf("⚠ Sandbox image issue: %v", err)
	}

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

	_, err = br.QueueSubscribe(broker.SubjectPatchesSafe, "sandbox-group", func(msg *nats.Msg) {
		var valMsg broker.ValidatedPatchMsg
		if err := json.Unmarshal(msg.Data, &valMsg); err != nil {
			log.Printf("❌ Failed to parse validated patch: %v", err)
			return
		}

		go func() {
			jobID := valMsg.JobID
			name := fmt.Sprintf("%s/%s", valMsg.Patch.Provider, valMsg.Patch.Model)

			// Fetch job details
			job, err := db.GetJob(jobID)
			if err != nil {
				log.Printf("[job:%s] Failed to fetch job: %v", jobID, err)
				return
			}

			// Parse GitHub repository URL to build CloneURL
			owner, repo, _, err := gh.ParseIssueURL(job.IssueURL)
			if err != nil {
				log.Printf("[job:%s] Failed to parse issue URL: %v", jobID, err)
				return
			}
			cloneURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

			emit(jobID, "🐳 **Phase 2/4: Sandbox Execution** — Docker verification...")
			emit(jobID, fmt.Sprintf("  🔄 Testing %s in sandbox...", name))

			testScript := sandbox.BuildTestScriptForLanguage(cloneURL, job.Language)
			result, err := sb.RunVerification(valMsg.Patch.Code, testScript)

			var resultMsg broker.SandboxResultMsg
			resultMsg.JobID = jobID
			resultMsg.Patch = valMsg.Patch

			if err != nil {
				log.Printf("[job:%s] Sandbox run failed with error: %v", jobID, err)
				resultMsg.SandboxResult = &sandbox.Result{Success: false, Logs: err.Error()}
				resultMsg.Eliminated = true
				resultMsg.SandboxScore = 0
				emit(jobID, fmt.Sprintf("  ❌ %s — sandbox error: %v", name, err))
			} else {
				resultMsg.SandboxResult = result
				if !result.Success {
					resultMsg.Eliminated = true
					resultMsg.SandboxScore = 0
					emit(jobID, fmt.Sprintf("  ❌ %s — tests failed (exit %d)", name, result.ExitCode))
				} else {
					resultMsg.Eliminated = false
					resultMsg.SandboxScore = scorePerformance(result)
					emit(jobID, fmt.Sprintf("  ✅ %s — tests passed (%.0fms, score: %.1f)", name, float64(result.DurationMs), resultMsg.SandboxScore))
				}
			}

			data, _ := json.Marshal(resultMsg)
			if err := br.Publish(broker.SubjectSandboxResults, data); err != nil {
				log.Printf("[job:%s] Failed to publish sandbox result: %v", jobID, err)
			}
		}()
	})

	if err != nil {
		log.Fatalf("❌ Failed to subscribe to raven.patches.safe: %v", err)
	}

	log.Println("✓ Sandbox Agent ready and listening on subject", broker.SubjectPatchesSafe)

	// Keep alive
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}

func scorePerformance(result *sandbox.Result) float64 {
	if !result.Success {
		return 0
	}
	bonus := 30.0
	ms := float64(result.DurationMs)
	if ms > 30000 {
		bonus = 0
	} else if ms > 5000 {
		bonus = 30.0 * (1.0 - (ms-5000)/25000)
	}
	return 70.0 + bonus
}
