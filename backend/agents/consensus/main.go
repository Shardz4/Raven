package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/Shardz4/raven/broker"
	"github.com/Shardz4/raven/config"
	"github.com/Shardz4/raven/consensus"
	gh "github.com/Shardz4/raven/github"
	"github.com/Shardz4/raven/llm"
	"github.com/Shardz4/raven/store"
	"github.com/nats-io/nats.go"
)

type JobState struct {
	Candidates []*consensus.Candidate
	Expected   int
	Round      int
}

var (
	stateMu sync.Mutex
	states  = make(map[string]*JobState)
)

func main() {
	log.Println("🪶 Raven Consensus Agent Starting...")

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

	// Forward declaration of runConsensus so handleCandidate can reference it
	var runConsensus func(jobID string, candidates []*consensus.Candidate, round int)

	handleCandidate := func(jobID string, candidate *consensus.Candidate) {
		stateMu.Lock()
		state, ok := states[jobID]
		if !ok {
			// First result, initialize state
			providers := cfg.AvailableProviders()
			expected := len(providers)
			if expected > cfg.Redundancy {
				expected = cfg.Redundancy
			}
			state = &JobState{
				Candidates: []*consensus.Candidate{},
				Expected:   expected,
				Round:      0,
			}
			states[jobID] = state

			// Start fallback timeout (e.g. 5 minutes)
			go func(jid string) {
				time.Sleep(5 * time.Minute)
				stateMu.Lock()
				st, exists := states[jid]
				if exists && len(st.Candidates) > 0 {
					log.Printf("[job:%s] Timeout hit, running consensus with %d/%d candidates", jid, len(st.Candidates), st.Expected)
					delete(states, jid)
					stateMu.Unlock()
					runConsensus(jid, st.Candidates, st.Round)
				} else {
					stateMu.Unlock()
				}
			}(jobID)
		}

		state.Candidates = append(state.Candidates, candidate)
		log.Printf("[job:%s] Collected candidate %d/%d", jobID, len(state.Candidates), state.Expected)

		if len(state.Candidates) == state.Expected {
			candidates := state.Candidates
			round := state.Round
			delete(states, jobID) // Remove from active states
			stateMu.Unlock()

			runConsensus(jobID, candidates, round)
		} else {
			stateMu.Unlock()
		}
	}

	runConsensus = func(jobID string, candidates []*consensus.Candidate, round int) {
		log.Printf("[job:%s] Running consensus (round %d) on %d candidates...", jobID, round, len(candidates))

		// Get job
		job, err := db.GetJob(jobID)
		if err != nil {
			log.Printf("[job:%s] Failed to get job: %v", jobID, err)
			return
		}

		// Build providers to get the judge
		_, judge, err := llm.BuildProviders(cfg)
		if err != nil {
			log.Printf("[job:%s] BuildProviders failed: %v", jobID, err)
			judge = nil
		}

		emit(jobID, "🧠 **Starting RavenMind Consensus Engine...**")
		engine := consensus.NewEngine(nil, judge, nil, 0, func(eventMsg string) {
			emit(jobID, eventMsg)
		})

		report := engine.EvaluateDistributed(candidates)

		if report.Winner == nil {
			// All failed safety or sandbox
			// Self-healing check
			if cfg.MaxHealRetries > 0 && round < cfg.MaxHealRetries {
				newRound := round + 1
				emit(jobID, fmt.Sprintf("🔄 **Self-Healing Round %d/%d** — Feeding errors back to LLMs...", newRound, cfg.MaxHealRetries))

				var errorFeedback strings.Builder
				errorFeedback.WriteString("Your previous code patch FAILED testing. Here are the errors:\n\n")
				for _, c := range candidates {
					if c.SandboxResult != nil && !c.SandboxResult.Success {
						errorFeedback.WriteString(fmt.Sprintf("=== %s/%s (exit code %d) ===\n%s\n\n",
							c.Patch.Provider, c.Patch.Model, c.SandboxResult.ExitCode,
							truncate(c.SandboxResult.Logs, 2000)))
					}
				}
				errorFeedback.WriteString("\nPlease fix your code based on these errors. Return ONLY the corrected code in a markdown code block.")

				healPrompt := errorFeedback.String()

				// Fetch issue to get original language
				issue, err := fetcher.FetchIssue(job.IssueURL)
				if err != nil {
					log.Printf("[job:%s] Self-healing fetch issue failed: %v", jobID, err)
					job.Status = "failed"
					job.ErrorMessage = "Self-healing failed: issue fetch failed"
					_ = db.UpdateJobResult(job)
					emit(jobID, "❌ "+job.ErrorMessage)
					emit(jobID, "[DONE]")
					return
				}

				// Re-initialize state for new round
				providers := cfg.AvailableProviders()
				expected := len(providers)
				if expected > cfg.Redundancy {
					expected = cfg.Redundancy
				}

				stateMu.Lock()
				states[jobID] = &JobState{
					Candidates: []*consensus.Candidate{},
					Expected:   expected,
					Round:      newRound,
				}
				// Re-start fallback timeout
				go func(jid string, exp int, rnd int) {
					time.Sleep(5 * time.Minute)
					stateMu.Lock()
					st, exists := states[jid]
					if exists && len(st.Candidates) > 0 {
						log.Printf("[job:%s] Timeout hit in round %d, running consensus with %d/%d candidates", jid, rnd, len(st.Candidates), st.Expected)
						delete(states, jid)
						stateMu.Unlock()
						runConsensus(jid, st.Candidates, rnd)
					} else {
						stateMu.Unlock()
					}
				}(jobID, expected, newRound)
				stateMu.Unlock()

				// Publish solver requests
				selected := providers
				if len(selected) > cfg.Redundancy {
					selected = selected[:cfg.Redundancy]
				}

				for _, provider := range selected {
					model := getModelForProvider(provider, cfg)
					patchReq := broker.PatchRequest{
						JobID:    jobID,
						Provider: provider,
						Model:    model,
						Prompt:   healPrompt,
						Language: issue.Language,
					}
					reqData, _ := json.Marshal(patchReq)
					_ = br.Publish(broker.SubjectSolverPrefix+provider, reqData)
					emit(jobID, fmt.Sprintf("⚡ Prompting %s/%s (healed)...", provider, model))
				}
				return
			}

			// Healing failed or disabled
			job.Status = "failed"
			job.ErrorMessage = report.Summary
			job.VerificationLogs = report.Summary
			_ = db.UpdateJobResult(job)
			emit(jobID, "❌ No patches survived consensus")
			emit(jobID, "[DONE]")

			// Record all participants as losses
			for _, c := range report.Candidates {
				if c.Patch != nil {
					model := fmt.Sprintf("%s/%s", c.Patch.Provider, c.Patch.Model)
					_ = db.RecordResult(model, false, 0)
				}
			}
			return
		}

		// We have a winner!
		winnerModel := fmt.Sprintf("%s/%s", report.Winner.Patch.Provider, report.Winner.Patch.Model)
		for _, c := range report.Candidates {
			if c.Patch != nil {
				model := fmt.Sprintf("%s/%s", c.Patch.Provider, c.Patch.Model)
				isWinner := model == winnerModel
				_ = db.RecordResult(model, isWinner, c.FinalScore)
			}
		}

		reportJSON, _ := json.Marshal(report)
		job.Status = "completed"
		job.WinnerModel = winnerModel
		job.WinnerCode = report.Winner.Patch.Code
		job.Explanation = report.Winner.Patch.Explanation
		job.ConsensusReport = reportJSON
		job.VerificationLogs = report.Summary
		_ = db.UpdateJobResult(job)

		emit(jobID, fmt.Sprintf("✅ Resolution complete! Winner: %s (score: %.1f)", job.WinnerModel, report.Winner.FinalScore))

		// Publish winner details to PR Agent
		reportBytes, _ := json.Marshal(report)
		winnerMsg := broker.ConsensusWinnerMsg{
			JobID:  jobID,
			Report: reportBytes,
		}
		winnerData, _ := json.Marshal(winnerMsg)
		_ = br.Publish(broker.SubjectConsensusWinner, winnerData)

		emit(jobID, "[DONE]")
	}

	// Subscribe to sandbox results
	_, err = br.Subscribe(broker.SubjectSandboxResults, func(msg *nats.Msg) {
		var sandboxMsg broker.SandboxResultMsg
		if err := json.Unmarshal(msg.Data, &sandboxMsg); err != nil {
			log.Printf("❌ Failed to parse sandbox result: %v", err)
			return
		}

		candidate := &consensus.Candidate{
			Patch:         sandboxMsg.Patch,
			SandboxResult: sandboxMsg.SandboxResult,
			SandboxScore:  sandboxMsg.SandboxScore,
			Eliminated:    sandboxMsg.Eliminated,
			Blocked:       false,
		}

		handleCandidate(sandboxMsg.JobID, candidate)
	})
	if err != nil {
		log.Fatalf("❌ Failed to subscribe to sandbox results: %v", err)
	}

	// Subscribe to blocked results
	_, err = br.Subscribe(broker.SubjectPatchesBlocked, func(msg *nats.Msg) {
		var valMsg broker.ValidatedPatchMsg
		if err := json.Unmarshal(msg.Data, &valMsg); err != nil {
			log.Printf("❌ Failed to parse blocked patch: %v", err)
			return
		}

		candidate := &consensus.Candidate{
			Patch:        valMsg.Patch,
			SafetyResult: valMsg.SafetyResult,
			Blocked:      true,
		}

		handleCandidate(valMsg.JobID, candidate)
	})
	if err != nil {
		log.Fatalf("❌ Failed to subscribe to blocked patches: %v", err)
	}

	log.Println("✓ Consensus Agent ready and listening on sandbox results and blocked patches...")

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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
