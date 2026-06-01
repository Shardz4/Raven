package main

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Shardz4/raven/config"
	"github.com/Shardz4/raven/store"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func main() {
	log.Println("🪶 Raven Store Service Starting...")

	cfg := config.Load()
	db, err := store.New(cfg.DBPath)
	if err != nil {
		log.Fatalf("❌ Database init failed: %v", err)
	}
	defer db.Close()

	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	r.Post("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		var job store.Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.CreateJob(&job); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	})

	r.Put("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		var job store.Job
		if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.UpdateJobResult(&job); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/api/jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		job, err := db.GetJob(id)
		if err != nil {
			http.Error(w, "job not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(job)
	})

	r.Get("/api/jobs", func(w http.ResponseWriter, r *http.Request) {
		limitStr := r.URL.Query().Get("limit")
		limit := 50
		if limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil {
				limit = l
			}
		}
		jobs, err := db.ListJobs(limit)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if jobs == nil {
			jobs = []*store.Job{}
		}
		json.NewEncoder(w).Encode(jobs)
	})

	r.Post("/api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model string  `json:"model"`
			Won   bool    `json:"won"`
			Score float64 `json:"score"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := db.RecordResult(payload.Model, payload.Won, payload.Score); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	r.Get("/api/leaderboard", func(w http.ResponseWriter, r *http.Request) {
		entries, err := db.GetLeaderboard()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if entries == nil {
			entries = []*store.LeaderboardEntry{}
		}
		json.NewEncoder(w).Encode(entries)
	})

	// Add port configuration specifically for the store agent
	port := "8081"
	log.Printf("🚀 Store Service listening on http://localhost:%s", port)
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
	}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("❌ Store Service failed: %v", err)
	}
}
