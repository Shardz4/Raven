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
	"github.com/Shardz4/raven/store"
	"github.com/Shardz4/raven/validation"
	"github.com/nats-io/nats.go"
)

func main() {
	log.Println("🪶 Raven Safety Agent Starting...")

	cfg := config.Load()
	db := store.NewClient(cfg.StoreServiceURL)

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

	_, err = br.QueueSubscribe(broker.SubjectPatches, "safety-group", func(msg *nats.Msg) {
		var patchMsg broker.PatchResultMsg
		if err := json.Unmarshal(msg.Data, &patchMsg); err != nil {
			log.Printf("❌ Failed to parse patch result message: %v", err)
			return
		}

		go func() {
			jobID := patchMsg.JobID

			// Check for solver errors
			if patchMsg.Error != "" {
				valMsg := broker.ValidatedPatchMsg{
					JobID:   jobID,
					Patch:   patchMsg.Patch,
					Blocked: true,
					Error:   patchMsg.Error,
				}
				data, _ := json.Marshal(valMsg)
				_ = br.Publish(broker.SubjectPatchesBlocked, data)
				return
			}

			if patchMsg.Patch == nil {
				log.Printf("[job:%s] Warning: Received empty patch from solver", jobID)
				return
			}

			// Get job to know the language
			job, err := db.GetJob(jobID)
			if err != nil {
				log.Printf("[job:%s] Failed to fetch job: %v", jobID, err)
				return
			}

			emit(jobID, "🛡️ **Phase 1/4: Safety Gate** — Static analysis...")

			var safetyResult *validation.Result
			language := job.Language
			switch language {
			case "go", "golang":
				safetyResult = validation.ValidateGoCode(patchMsg.Patch.Code)
			case "javascript", "typescript", "js", "ts", "rust":
				safetyResult = &validation.Result{OK: true, Reason: "OK"}
			default:
				safetyResult = validation.ValidatePythonPatch(patchMsg.Patch.Code)
			}

			valMsg := broker.ValidatedPatchMsg{
				JobID:        jobID,
				Patch:        patchMsg.Patch,
				SafetyResult: safetyResult,
				Blocked:      !safetyResult.OK,
			}

			data, _ := json.Marshal(valMsg)
			name := fmt.Sprintf("%s/%s", patchMsg.Patch.Provider, patchMsg.Patch.Model)

			if !safetyResult.OK {
				emit(jobID, fmt.Sprintf("  ⛔ %s blocked: %s", name, safetyResult.Reason))
				if err := br.Publish(broker.SubjectPatchesBlocked, data); err != nil {
					log.Printf("[job:%s] Failed to publish blocked patch: %v", jobID, err)
				}
			} else {
				emit(jobID, fmt.Sprintf("  ✅ %s passed safety gate", name))
				if err := br.Publish(broker.SubjectPatchesSafe, data); err != nil {
					log.Printf("[job:%s] Failed to publish safe patch: %v", jobID, err)
				}
			}
		}()
	})

	if err != nil {
		log.Fatalf("❌ Failed to subscribe to raven.patches: %v", err)
	}

	log.Println("✓ Safety Agent ready and listening on subject", broker.SubjectPatches)

	// Keep alive
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
}
