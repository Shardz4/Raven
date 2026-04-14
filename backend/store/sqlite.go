package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Job represents a single issue resolution job persisted in the database.
type Job struct {
	ID               string          `json:"id"`
	IssueURL         string          `json:"issue_url"`
	IssueTitle       string          `json:"issue_title"`
	Status           string          `json:"status"` // "pending", "running", "completed", "failed"
	WinnerModel      string          `json:"winner_model,omitempty"`
	WinnerCode       string          `json:"winner_code,omitempty"`
	Explanation      string          `json:"explanation,omitempty"`
	ConsensusReport  json.RawMessage `json:"consensus_report,omitempty"`
	VerificationLogs string          `json:"verification_logs,omitempty"`
	ErrorMessage     string          `json:"error_message,omitempty"`
	PRURL            string          `json:"pr_url,omitempty"`
	Language         string          `json:"language,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	CompletedAt      *time.Time      `json:"completed_at,omitempty"`
}

// LeaderboardEntry represents a model's win statistics.
type LeaderboardEntry struct {
	Model    string  `json:"model"`
	Wins     int     `json:"wins"`
	Total    int     `json:"total"`
	WinRate  float64 `json:"win_rate"`
	AvgScore float64 `json:"avg_score"`
}

// Store wraps the SQLite database connection.
type Store struct {
	db *sql.DB
}

// New opens or creates the SQLite database at the given path and runs migrations.
func New(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Enable WAL mode for better concurrent read performance
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("set WAL: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS jobs (
		id               TEXT PRIMARY KEY,
		issue_url        TEXT NOT NULL,
		issue_title      TEXT NOT NULL DEFAULT '',
		status           TEXT NOT NULL DEFAULT 'pending',
		winner_model     TEXT NOT NULL DEFAULT '',
		winner_code      TEXT NOT NULL DEFAULT '',
		explanation      TEXT NOT NULL DEFAULT '',
		consensus_report TEXT NOT NULL DEFAULT '{}',
		verification_logs TEXT NOT NULL DEFAULT '',
		error_message    TEXT NOT NULL DEFAULT '',
		pr_url           TEXT NOT NULL DEFAULT '',
		language         TEXT NOT NULL DEFAULT 'python',
		created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		completed_at     DATETIME
	);
	CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
	CREATE INDEX IF NOT EXISTS idx_jobs_created ON jobs(created_at);

	CREATE TABLE IF NOT EXISTS leaderboard (
		model       TEXT NOT NULL,
		wins        INTEGER NOT NULL DEFAULT 0,
		total       INTEGER NOT NULL DEFAULT 0,
		total_score REAL NOT NULL DEFAULT 0,
		PRIMARY KEY (model)
	);
	`
	_, err := s.db.Exec(schema)
	return err
}

// CreateJob inserts a new job record.
func (s *Store) CreateJob(job *Job) error {
	_, err := s.db.Exec(
		`INSERT INTO jobs (id, issue_url, issue_title, status, created_at) VALUES (?, ?, ?, ?, ?)`,
		job.ID, job.IssueURL, job.IssueTitle, job.Status, job.CreatedAt,
	)
	return err
}

// UpdateJobResult updates a job with its final resolution result.
func (s *Store) UpdateJobResult(job *Job) error {
	report, _ := json.Marshal(job.ConsensusReport)
	now := time.Now()
	_, err := s.db.Exec(
		`UPDATE jobs SET status=?, winner_model=?, winner_code=?, explanation=?,
		 consensus_report=?, verification_logs=?, error_message=?, completed_at=?
		 WHERE id=?`,
		job.Status, job.WinnerModel, job.WinnerCode, job.Explanation,
		string(report), job.VerificationLogs, job.ErrorMessage, now, job.ID,
	)
	return err
}

// GetJob retrieves a single job by ID.
func (s *Store) GetJob(id string) (*Job, error) {
	row := s.db.QueryRow(`SELECT id, issue_url, issue_title, status, winner_model, winner_code,
		explanation, consensus_report, verification_logs, error_message, created_at, completed_at
		FROM jobs WHERE id=?`, id)
	return scanJob(row)
}

// ListJobs returns the most recent jobs, limited to `limit`.
func (s *Store) ListJobs(limit int) ([]*Job, error) {
	rows, err := s.db.Query(`SELECT id, issue_url, issue_title, status, winner_model, winner_code,
		explanation, consensus_report, verification_logs, error_message, created_at, completed_at
		FROM jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// RecordResult records a model's participation and whether it won.
func (s *Store) RecordResult(model string, won bool, score float64) error {
	_, err := s.db.Exec(`
		INSERT INTO leaderboard (model, wins, total, total_score)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(model) DO UPDATE SET
			wins = wins + ?,
			total = total + 1,
			total_score = total_score + ?`,
		model, boolToInt(won), score, boolToInt(won), score)
	return err
}

// GetLeaderboard returns all models ranked by win rate.
func (s *Store) GetLeaderboard() ([]*LeaderboardEntry, error) {
	rows, err := s.db.Query(`
		SELECT model, wins, total, 
		       CASE WHEN total > 0 THEN CAST(wins AS REAL) / total ELSE 0 END as win_rate,
		       CASE WHEN total > 0 THEN total_score / total ELSE 0 END as avg_score
		FROM leaderboard
		ORDER BY wins DESC, win_rate DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*LeaderboardEntry
	for rows.Next() {
		e := &LeaderboardEntry{}
		if err := rows.Scan(&e.Model, &e.Wins, &e.Total, &e.WinRate, &e.AvgScore); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// scanner interface for both sql.Row and sql.Rows
func scanJob(row *sql.Row) (*Job, error) {
	j := &Job{}
	var consensusStr string
	var completedAt sql.NullTime
	err := row.Scan(
		&j.ID, &j.IssueURL, &j.IssueTitle, &j.Status, &j.WinnerModel, &j.WinnerCode,
		&j.Explanation, &consensusStr, &j.VerificationLogs, &j.ErrorMessage, &j.CreatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	j.ConsensusReport = json.RawMessage(consensusStr)
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return j, nil
}

func scanJobRow(rows *sql.Rows) (*Job, error) {
	j := &Job{}
	var consensusStr string
	var completedAt sql.NullTime
	err := rows.Scan(
		&j.ID, &j.IssueURL, &j.IssueTitle, &j.Status, &j.WinnerModel, &j.WinnerCode,
		&j.Explanation, &consensusStr, &j.VerificationLogs, &j.ErrorMessage, &j.CreatedAt, &completedAt,
	)
	if err != nil {
		return nil, err
	}
	j.ConsensusReport = json.RawMessage(consensusStr)
	if completedAt.Valid {
		j.CompletedAt = &completedAt.Time
	}
	return j, nil
}
