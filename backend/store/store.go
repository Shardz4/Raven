package store

// Storer defines the database operations needed by Raven agents.
type Storer interface {
	CreateJob(job *Job) error
	UpdateJobResult(job *Job) error
	GetJob(id string) (*Job, error)
	ListJobs(limit int) ([]*Job, error)
	RecordResult(model string, won bool, score float64) error
	GetLeaderboard() ([]*LeaderboardEntry, error)
	Close() error
}
