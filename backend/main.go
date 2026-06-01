package main

import (
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/Shardz4/raven/api"
	"github.com/Shardz4/raven/bots"
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

	// 2. Initialize store based on AgentMode
	var db store.Storer
	var err error
	if cfg.AgentMode == "distributed" {
		db = store.NewClient(cfg.StoreServiceURL)
		log.Println("✓ Database client ready (Store Service)")
	} else {
		var localDB *store.Store
		localDB, err = store.New(cfg.DBPath)
		if err != nil {
			log.Fatalf("❌ Database init failed: %v", err)
		}
		db = localDB
		log.Println("✓ Database ready (local SQLite)")
	}
	defer db.Close()

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

	// 7. Start chat bots (if tokens configured)
	botSvc := bots.NewService(server, cfg)
	if cfg.TelegramToken != "" {
		go bots.StartTelegram(cfg.TelegramToken, botSvc)
		log.Println("✓ Telegram bot started")
	} else {
		log.Println("⚠ Telegram bot disabled (no TELEGRAM_BOT_TOKEN)")
	}
	if cfg.DiscordToken != "" {
		go bots.StartDiscord(cfg.DiscordToken, botSvc)
		log.Println("✓ Discord bot started")
	} else {
		log.Println("⚠ Discord bot disabled (no DISCORD_BOT_TOKEN)")
	}

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

