package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Shardz4/raven/broker"
	"github.com/Shardz4/raven/config"
	"github.com/Shardz4/raven/llm"
	"github.com/nats-io/nats.go"
)

func main() {
	log.Println("🪶 Raven Solver Agent Starting...")

	cfg := config.Load()
	providerName := os.Getenv("SOLVER_PROVIDER")
	if providerName == "" {
		log.Fatalf("❌ SOLVER_PROVIDER environment variable is required")
	}

	prov, err := llm.BuildProvider(providerName, cfg)
	if err != nil {
		log.Fatalf("❌ Failed to build provider %s: %v", providerName, err)
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

	subject := broker.SubjectSolverPrefix + providerName
	queueGroup := "solver-group-" + providerName

	_, err = br.QueueSubscribe(subject, queueGroup, func(msg *nats.Msg) {
		var req broker.PatchRequest
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			log.Printf("❌ Failed to parse patch request: %v", err)
			return
		}

		go func() {
			name := fmt.Sprintf("%s/%s", prov.Name(), prov.Model())
			start := time.Now()

			result, err := prov.GeneratePatch(req.Prompt)
			durationMs := time.Since(start).Milliseconds()

			var resultMsg broker.PatchResultMsg
			resultMsg.JobID = req.JobID

			if err != nil {
				log.Printf("[job:%s] Solver %s failed: %v", req.JobID, name, err)
				emit(req.JobID, fmt.Sprintf("❌ %s failed: %v", name, err))
				resultMsg.Error = err.Error()
			} else {
				result.DurationMs = durationMs
				resultMsg.Patch = result
				emit(req.JobID, fmt.Sprintf("📦 Received patch from %s (%dms)", name, durationMs))
			}

			data, _ := json.Marshal(resultMsg)
			if err := br.Publish(broker.SubjectPatches, data); err != nil {
				log.Printf("[job:%s] Failed to publish patch result: %v", req.JobID, err)
			}
		}()
	})

	if err != nil {
		log.Fatalf("❌ Failed to subscribe to NATS solver channel: %v", err)
	}

	log.Printf("✓ Solver Agent for %s/%s ready and listening on subject %s...", providerName, prov.Model(), subject)

	// Keep alive
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
