package broker

import (
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// Broker wraps the NATS connection.
type Broker struct {
	nc *nats.Conn
	js nats.JetStreamContext
}

// New connects to NATS and returns a Broker instance.
func New(url string) (*Broker, error) {
	opts := []nats.Option{
		nats.Name("Raven Agent"),
		nats.Timeout(10 * time.Second),
		nats.ReconnectWait(2 * time.Second),
		nats.MaxReconnects(5),
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	// Initialize JetStream context
	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("jetstream init: %w", err)
	}

	b := &Broker{
		nc: nc,
		js: js,
	}

	// Setup streams
	if err := b.setupStreams(); err != nil {
		nc.Close()
		return nil, fmt.Errorf("setup streams: %w", err)
	}

	return b, nil
}

// setupStreams ensures the required JetStream streams exist.
func (b *Broker) setupStreams() error {
	// Define streams to cover raven.* subjects
	streams := []struct {
		name     string
		subjects []string
	}{
		{
			name:     "RAVEN_JOBS",
			subjects: []string{SubjectJobs},
		},
		{
			name:     "RAVEN_SOLVERS",
			subjects: []string{SubjectSolverPrefix + "*"},
		},
		{
			name:     "RAVEN_PATCHES",
			subjects: []string{SubjectPatches, SubjectPatchesSafe, SubjectPatchesBlocked},
		},
		{
			name:     "RAVEN_RESULTS",
			subjects: []string{SubjectSandboxResults, SubjectConsensusWinner},
		},
	}

	for _, s := range streams {
		cfg := &nats.StreamConfig{
			Name:     s.name,
			Subjects: s.subjects,
			Storage:  nats.MemoryStorage, // Use memory storage for ephemeral agent messages
		}

		// Check if stream exists first
		_, err := b.js.StreamInfo(s.name)
		if err == nil {
			// Stream exists, update it
			_, err = b.js.UpdateStream(cfg)
			if err != nil {
				log.Printf("[broker] Warn: update stream %s failed: %v", s.name, err)
			}
		} else {
			// Create new stream
			_, err = b.js.AddStream(cfg)
			if err != nil {
				return fmt.Errorf("add stream %s: %w", s.name, err)
			}
		}
	}
	return nil
}

// Publish publishes a message on a NATS subject.
func (b *Broker) Publish(subject string, data []byte) error {
	_, err := b.js.Publish(subject, data)
	return err
}

// Subscribe subscribes to a subject with a handler function using JetStream push subscription.
func (b *Broker) Subscribe(subject string, handler func(msg *nats.Msg)) (*nats.Subscription, error) {
	sub, err := b.js.Subscribe(subject, handler)
	if err != nil {
		return nil, fmt.Errorf("subscribe %s: %w", subject, err)
	}
	return sub, nil
}

// QueueSubscribe subscribes to a subject using a queue group for load balancing.
func (b *Broker) QueueSubscribe(subject, queue string, handler func(msg *nats.Msg)) (*nats.Subscription, error) {
	sub, err := b.js.QueueSubscribe(subject, queue, handler)
	if err != nil {
		return nil, fmt.Errorf("queue subscribe %s/%s: %w", subject, queue, err)
	}
	return sub, nil
}

// Close closes the underlying NATS connection.
func (b *Broker) Close() {
	b.nc.Close()
}
