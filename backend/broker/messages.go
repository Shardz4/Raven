package broker

import (
	"encoding/json"

	"github.com/Shardz4/raven/llm"
	"github.com/Shardz4/raven/sandbox"
	"github.com/Shardz4/raven/validation"
)

// JobRequest is published to start a new job.
type JobRequest struct {
	JobID    string `json:"job_id"`
	IssueURL string `json:"issue_url"`
}

// PatchRequest is published to prompt a specific solver.
type PatchRequest struct {
	JobID    string `json:"job_id"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Language string `json:"language"`
}

// PatchResultMsg wraps a solver's result for NATS.
type PatchResultMsg struct {
	JobID string            `json:"job_id"`
	Patch *llm.PatchResult `json:"patch"`
	Error string            `json:"error,omitempty"`
}

// ValidatedPatchMsg is published after safety validation.
type ValidatedPatchMsg struct {
	JobID        string             `json:"job_id"`
	Patch        *llm.PatchResult   `json:"patch"`
	SafetyResult *validation.Result `json:"safety_result"`
	Blocked      bool               `json:"blocked"`
	Error        string             `json:"error,omitempty"`
}

// SandboxRequest is published to trigger Docker sandbox runs.
type SandboxRequest struct {
	JobID      string             `json:"job_id"`
	Language   string             `json:"language"`
	TestScript string             `json:"test_script"`
	Patches    []*llm.PatchResult `json:"patches"`
}

// SandboxResultMsg wraps the sandbox execution results.
type SandboxResultMsg struct {
	JobID         string             `json:"job_id"`
	SandboxScore  float64            `json:"sandbox_score"`
	SandboxResult *sandbox.Result    `json:"sandbox_result"`
	Patch         *llm.PatchResult   `json:"patch"`
	Eliminated    bool               `json:"eliminated"`
}

// ConsensusRequest triggers final scoring.
type ConsensusRequest struct {
	JobID            string              `json:"job_id"`
	Language         string              `json:"language"`
	TestScript       string              `json:"test_script"`
	SandboxResults   []*SandboxResultMsg `json:"sandbox_results"`
}

// EventMsg represents live progress logs sent over NATS.
type EventMsg struct {
	JobID   string `json:"job_id"`
	Message string `json:"message"`
}

// ConsensusWinnerMsg is published when a consensus winner is selected.
type ConsensusWinnerMsg struct {
	JobID  string          `json:"job_id"`
	Report json.RawMessage `json:"report"`
}
