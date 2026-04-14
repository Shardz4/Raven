package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Shardz4/raven/api"
	"github.com/Shardz4/raven/config"
	gh "github.com/Shardz4/raven/github"
	"github.com/Shardz4/raven/llm"
	"github.com/Shardz4/raven/sandbox"
	"github.com/Shardz4/raven/store"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("🪶 Raven v2.0 — Starting backend...")

	// 1. Load config
	cfg := config.Load()

	// 2. Initialize SQLite store
	db, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Database init failed: %v", err)
	}
	defer db.Close()
	log.Println("✓ Database ready")

	// 3. Initialize GitHub fetcher + PR creator
	fetcher := gh.NewFetcher(cfg.GitHubToken)
	prCreator := gh.NewPRCreator(cfg.GitHubToken)
	log.Println("✓ GitHub fetcher ready")
	if prCreator.CanCreatePR() {
		log.Println("✓ Auto-PR enabled (GITHUB_TOKEN set)")
	} else {
		log.Println("⚠ Auto-PR disabled (no GITHUB_TOKEN)")
	}

	// 4. Initialize LLM providers
	solvers, judge, err := llm.BuildProviders(cfg)
	if err != nil {
		log.Fatalf("❌ LLM init failed: %v", err)
	}
	log.Printf("✓ %d solver(s) + judge (%s/%s) ready", len(solvers), judge.Name(), judge.Model())

	// 5. Initialize Docker sandbox
	sb, err := sandbox.NewManager(cfg.SandboxImage, cfg.DockerTimeout)
	if err != nil {
		log.Printf("⚠ Docker sandbox not available: %v", err)
		log.Println("  Sandbox verification will be skipped. Start Docker Desktop to enable it.")
		sb = nil
	} else {
		defer sb.Close()
		// Try to ensure the sandbox image exists
		if err := sb.EnsureImage("../sandbox_env"); err != nil {
			log.Printf("⚠ Sandbox image issue: %v", err)
		}
		log.Println("✓ Docker sandbox ready")
	}

	// 6. Start API server
	server := api.NewServer(cfg, db, fetcher, prCreator, solvers, judge, sb)
	router := server.Router()

	addr := ":" + cfg.Port
	log.Printf("🚀 Raven API server listening on http://localhost%s", addr)
	log.Printf("   POST /api/solve          — Submit a GitHub issue")
	log.Printf("   GET  /api/solve/{id}/stream — SSE event stream")
	log.Printf("   GET  /api/jobs           — List past jobs")
	log.Printf("   GET  /api/providers      — List configured LLMs")

	// Graceful shutdown on signals
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigCh
		log.Printf("Received %s, shutting down...", sig)
		os.Exit(0)
	}()

	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("❌ Server failed: %v", err)
	}
}
