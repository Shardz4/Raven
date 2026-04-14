package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Shardz4/raven/config"
	"github.com/Shardz4/raven/consensus"
	gh "github.com/Shardz4/raven/github"
	"github.com/Shardz4/raven/llm"
	"github.com/Shardz4/raven/sandbox"
	"github.com/Shardz4/raven/store"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/google/uuid"
)

// Server holds the API dependencies.
type Server struct {
	cfg      *config.Config
	store    *store.Store
	fetcher  *gh.Fetcher
	solvers  []llm.Provider
	judge    llm.Provider
	sandbox  *sandbox.Manager

	// SSE event channels per job ID
	mu       sync.RWMutex
	streams  map[string][]chan string
}

// NewServer creates the API server with all dependencies.
func NewServer(
	cfg *config.Config,
	db *store.Store,
	fetcher *gh.Fetcher,
	solvers []llm.Provider,
	judge llm.Provider,
	sb *sandbox.Manager,
) *Server {
	return &Server{
		cfg:     cfg,
		store:   db,
		fetcher: fetcher,
		solvers: solvers,
		judge:   judge,
		sandbox: sb,
		streams: make(map[string][]chan string),
	}
}

// Router returns the configured HTTP router.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(5 * time.Minute))
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type"},
		AllowCredentials: false,
	}))

	// Routes
	r.Get("/api/health", s.handleHealth)
	r.Post("/api/solve", s.handleSolve)
	r.Get("/api/solve/{id}", s.handleGetJob)
	r.Get("/api/solve/{id}/stream", s.handleStream)
	r.Get("/api/jobs", s.handleListJobs)
	r.Get("/api/providers", s.handleProviders)

	return r
}

// ── Handlers ──

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": "2.0.0",
	})
}

type solveRequest struct {
	IssueURL string `json:"issue_url"`
}

func (s *Server) handleSolve(w http.ResponseWriter, r *http.Request) {
	var req solveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if req.IssueURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "issue_url is required"})
		return
	}

	// Create job
	jobID := uuid.New().String()[:8]
	job := &store.Job{
		ID:       jobID,
		IssueURL: req.IssueURL,
		Status:   "pending",
		CreatedAt: time.Now(),
	}
	if err := s.store.CreateJob(job); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create job"})
		return
	}

	// Start processing in the background
	go s.processJob(job)

	writeJSON(w, http.StatusAccepted, map[string]any{
		"job_id":  jobID,
		"status":  "pending",
		"stream":  fmt.Sprintf("/api/solve/%s/stream", jobID),
		"result":  fmt.Sprintf("/api/solve/%s", jobID),
	})
}

func (s *Server) handleGetJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := s.store.GetJob(id)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "job not found"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := make(chan string, 100)

	// Register stream
	s.mu.Lock()
	s.streams[id] = append(s.streams[id], ch)
	s.mu.Unlock()

	// Cleanup on disconnect
	defer func() {
		s.mu.Lock()
		channels := s.streams[id]
		for i, c := range channels {
			if c == ch {
				s.streams[id] = append(channels[:i], channels[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleListJobs(w http.ResponseWriter, r *http.Request) {
	jobs, err := s.store.ListJobs(50)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if jobs == nil {
		jobs = []*store.Job{}
	}
	writeJSON(w, http.StatusOK, jobs)
}

func (s *Server) handleProviders(w http.ResponseWriter, r *http.Request) {
	providers := make([]map[string]string, 0, len(s.solvers))
	for _, p := range s.solvers {
		providers = append(providers, map[string]string{
			"name":  p.Name(),
			"model": p.Model(),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"solvers": providers,
		"judge": map[string]string{
			"name":  s.judge.Name(),
			"model": s.judge.Model(),
		},
	})
}

// ── Job Processing ──

func (s *Server) emitEvent(jobID, message string) {
	s.mu.RLock()
	channels := s.streams[jobID]
	s.mu.RUnlock()

	for _, ch := range channels {
		select {
		case ch <- message:
		default:
			// Drop if channel is full (slow consumer)
		}
	}
	log.Printf("[job:%s] %s", jobID, message)
}

func (s *Server) processJob(job *store.Job) {
	emit := func(msg string) { s.emitEvent(job.ID, msg) }

	job.Status = "running"
	_ = s.store.UpdateJobResult(job)

	// 1. Fetch the GitHub issue
	emit("🔍 Fetching GitHub issue...")
	issue, err := s.fetcher.FetchIssue(job.IssueURL)
	if err != nil {
		job.Status = "failed"
		job.ErrorMessage = fmt.Sprintf("Failed to fetch issue: %v", err)
		_ = s.store.UpdateJobResult(job)
		emit("❌ " + job.ErrorMessage)
		emit("[DONE]")
		return
	}
	job.IssueTitle = issue.Title
	emit(fmt.Sprintf("📋 Issue: %s", issue.Title))

	// 2. Build prompt from actual issue content
	prompt := issue.Prompt()

	// 3. Select providers (capped by redundancy config)
	selected := s.solvers
	if len(selected) > s.cfg.Redundancy {
		selected = selected[:s.cfg.Redundancy]
	}

	emit(fmt.Sprintf("📡 Engaging %d AI models in parallel...", len(selected)))

	// 4. Fan out to all LLMs concurrently
	patches := llm.FanOut(selected, prompt, emit)

	if len(patches) == 0 {
		job.Status = "failed"
		job.ErrorMessage = "No LLM returned a valid patch"
		_ = s.store.UpdateJobResult(job)
		emit("❌ " + job.ErrorMessage)
		emit("[DONE]")
		return
	}

	// 5. Build test script
	testScript := sandbox.BuildTestScript(issue.CloneURL)

	// 6. Run RavenMind Consensus
	emit("🧠 **Starting RavenMind Consensus Engine...**")
	engine := consensus.NewEngine(s.sandbox, s.judge, emit)
	report := engine.Evaluate(patches, testScript)

	if report.Winner == nil {
		job.Status = "failed"
		job.ErrorMessage = report.Summary
		job.VerificationLogs = report.Summary
		_ = s.store.UpdateJobResult(job)
		emit("❌ No patches survived consensus")
		emit("[DONE]")
		return
	}

	// 7. Save the result
	reportJSON, _ := json.Marshal(report)
	job.Status = "completed"
	job.WinnerModel = fmt.Sprintf("%s/%s", report.Winner.Patch.Provider, report.Winner.Patch.Model)
	job.WinnerCode = report.Winner.Patch.Code
	job.Explanation = report.Winner.Patch.Explanation
	job.ConsensusReport = reportJSON
	job.VerificationLogs = report.Summary
	_ = s.store.UpdateJobResult(job)

	emit(fmt.Sprintf("✅ Resolution complete! Winner: %s (score: %.1f)", job.WinnerModel, report.Winner.FinalScore))
	emit("[DONE]")
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
