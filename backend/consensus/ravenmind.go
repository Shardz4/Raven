// Package consensus implements the RavenMind multi-phase consensus engine.
//
// RavenMind is Raven's unique approach to selecting the best AI-generated code patch.
// Unlike simple majority-vote or first-success systems, it combines four independent
// evaluation phases into a weighted score:
//
//   Phase 1: Safety Gate        — static analysis blocks dangerous code (pass/fail)
//   Phase 2: Sandbox Execution  — Docker runs pytest; measures pass/fail + timing (35%)
//   Phase 3: Structural Similarity — AST fingerprint clustering rewards convergence (25%)
//   Phase 4: LLM Judge          — a separate model scores code quality 0-100 (40%)
//
// The weighted aggregate determines the winner.
package consensus

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Shardz4/raven/llm"
	"github.com/Shardz4/raven/sandbox"
	"github.com/Shardz4/raven/validation"
)

// Weights for each scoring phase.
const (
	WeightSandbox    = 0.35
	WeightStructural = 0.25
	WeightJudge      = 0.40
)

// Candidate is an LLM patch annotated with scores from each consensus phase.
type Candidate struct {
	Patch           *llm.PatchResult   `json:"patch"`
	SafetyResult    *validation.Result `json:"safety_result"`
	SandboxResult   *sandbox.Result    `json:"sandbox_result,omitempty"`
	SandboxScore    float64            `json:"sandbox_score"`    // 0-100
	StructuralScore float64            `json:"structural_score"` // 0-100
	JudgeScore      float64            `json:"judge_score"`      // 0-100
	FinalScore      float64            `json:"final_score"`      // Weighted aggregate
	Blocked         bool               `json:"blocked"`
	Eliminated      bool               `json:"eliminated"` // Failed sandbox
}

// Report is the full consensus output, including all candidates and the winner.
type Report struct {
	Candidates      []*Candidate `json:"candidates"`
	Winner          *Candidate   `json:"winner,omitempty"`
	TotalPatches    int          `json:"total_patches"`
	BlockedCount    int          `json:"blocked_count"`
	PassedSandbox   int          `json:"passed_sandbox"`
	UniqueStructures int         `json:"unique_structures"`
	Summary         string       `json:"summary"`
}

// Engine is the RavenMind consensus engine.
type Engine struct {
	sandbox    *sandbox.Manager
	judge      llm.Provider
	solvers    []llm.Provider // needed for self-healing retries
	maxRetries int            // max self-healing rounds
	onEvent    func(string)
}

// NewEngine creates a new RavenMind consensus engine.
// solvers are needed for self-healing (re-prompting on failure).
// maxRetries controls how many self-healing rounds to attempt (0 = disabled).
func NewEngine(sb *sandbox.Manager, judge llm.Provider, solvers []llm.Provider, maxRetries int, onEvent func(string)) *Engine {
	return &Engine{
		sandbox:    sb,
		judge:      judge,
		solvers:    solvers,
		maxRetries: maxRetries,
		onEvent:    onEvent,
	}
}

func (e *Engine) emit(msg string) {
	if e.onEvent != nil {
		e.onEvent(msg)
	}
}

// Evaluate runs all four phases on the given patches and returns the consensus report.
func (e *Engine) Evaluate(patches []*llm.PatchResult, testScript string) *Report {
	report := &Report{TotalPatches: len(patches)}

	// Build candidates
	candidates := make([]*Candidate, 0, len(patches))
	for _, p := range patches {
		candidates = append(candidates, &Candidate{Patch: p})
	}

	// ── Phase 1: Safety Gate ──
	e.emit("🛡️ **Phase 1/4: Safety Gate** — Static analysis...")
	for _, c := range candidates {
		c.SafetyResult = validation.ValidatePythonPatch(c.Patch.Code)
		if !c.SafetyResult.OK {
			c.Blocked = true
			report.BlockedCount++
			e.emit(fmt.Sprintf("  ⛔ %s/%s blocked: %s", c.Patch.Provider, c.Patch.Model, c.SafetyResult.Reason))
		} else {
			e.emit(fmt.Sprintf("  ✅ %s/%s passed safety gate", c.Patch.Provider, c.Patch.Model))
		}
	}

	// Filter to safe candidates
	safe := filterActive(candidates)
	if len(safe) == 0 {
		report.Summary = "All patches failed the safety gate"
		report.Candidates = candidates
		return report
	}

	// ── Phase 2: Sandbox Execution ──
	e.emit("🐳 **Phase 2/4: Sandbox Execution** — Docker verification...")
	for _, c := range safe {
		name := fmt.Sprintf("%s/%s", c.Patch.Provider, c.Patch.Model)
		e.emit(fmt.Sprintf("  🔄 Testing %s in sandbox...", name))

		result, err := e.sandbox.RunVerification(c.Patch.Code, testScript)
		if err != nil {
			c.SandboxResult = &sandbox.Result{Success: false, Logs: err.Error()}
			c.Eliminated = true
			e.emit(fmt.Sprintf("  ❌ %s — sandbox error: %v", name, err))
			continue
		}

		c.SandboxResult = result
		if !result.Success {
			c.Eliminated = true
			c.SandboxScore = 0
			e.emit(fmt.Sprintf("  ❌ %s — tests failed (exit %d)", name, result.ExitCode))
		} else {
			// Score: base 70 for passing, up to 100 based on speed
			c.SandboxScore = scoreSandboxPerformance(result)
			report.PassedSandbox++
			e.emit(fmt.Sprintf("  ✅ %s — tests passed (%.0fms, score: %.1f)", name, float64(result.DurationMs), c.SandboxScore))
		}
	}

	// Filter to passing candidates
	passing := filterPassing(candidates)
	if len(passing) == 0 {
		// ── Self-Healing: retry with error feedback ──
		if e.maxRetries > 0 && e.solvers != nil && len(e.solvers) > 0 {
			return e.selfHeal(candidates, testScript, report, 1)
		}
		report.Summary = "All patches failed sandbox verification"
		report.Candidates = candidates
		return report
	}

	// ── Phase 3: Structural Similarity ──
	e.emit("🧬 **Phase 3/4: Structural Similarity** — AST fingerprinting...")
	fingerprints := map[string][]*Candidate{}
	for _, c := range passing {
		fp := validation.StructuralFingerprint(c.Patch.Code)
		fingerprints[fp] = append(fingerprints[fp], c)
	}
	report.UniqueStructures = len(fingerprints)

	// Score: candidates in larger clusters get higher scores
	maxCluster := 0
	for _, cluster := range fingerprints {
		if len(cluster) > maxCluster {
			maxCluster = len(cluster)
		}
	}
	for fp, cluster := range fingerprints {
		score := 100.0
		if maxCluster > 1 {
			score = (float64(len(cluster)) / float64(maxCluster)) * 100.0
		}
		for _, c := range cluster {
			c.StructuralScore = score
		}
		e.emit(fmt.Sprintf("  📊 Cluster '%s...' — %d members, score: %.1f", truncate(fp, 30), len(cluster), score))
	}

	// ── Phase 4: LLM Judge ──
	e.emit("⚖️ **Phase 4/4: LLM Judge** — Quality evaluation...")
	e.runJudgePhase(passing)

	// ── Aggregate Final Scores ──
	e.emit("🧮 **Scoring** — Computing weighted consensus...")
	for _, c := range passing {
		c.FinalScore = (c.SandboxScore * WeightSandbox) +
			(c.StructuralScore * WeightStructural) +
			(c.JudgeScore * WeightJudge)
	}

	// Sort by final score descending
	sort.Slice(passing, func(i, j int) bool {
		return passing[i].FinalScore > passing[j].FinalScore
	})

	report.Winner = passing[0]
	report.Candidates = candidates

	// Build summary
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== RAVENMIND CONSENSUS ===\n"))
	sb.WriteString(fmt.Sprintf("Total patches: %d | Blocked: %d | Passed Sandbox: %d | Unique structures: %d\n\n",
		report.TotalPatches, report.BlockedCount, report.PassedSandbox, report.UniqueStructures))

	for i, c := range passing {
		sb.WriteString(fmt.Sprintf("%d. %s/%s — Final: %.1f (Sandbox: %.1f × %.0f%% + Structural: %.1f × %.0f%% + Judge: %.1f × %.0f%%)\n",
			i+1, c.Patch.Provider, c.Patch.Model, c.FinalScore,
			c.SandboxScore, WeightSandbox*100,
			c.StructuralScore, WeightStructural*100,
			c.JudgeScore, WeightJudge*100))
	}
	sb.WriteString(fmt.Sprintf("\n🏆 Winner: %s/%s (score: %.1f)\n", report.Winner.Patch.Provider, report.Winner.Patch.Model, report.Winner.FinalScore))
	report.Summary = sb.String()

	e.emit(fmt.Sprintf("🏆 **Winner:** %s/%s with score %.1f", report.Winner.Patch.Provider, report.Winner.Patch.Model, report.Winner.FinalScore))
	return report
}

// runJudgePhase asks the judge LLM to evaluate all passing patches.
func (e *Engine) runJudgePhase(candidates []*Candidate) {
	if e.judge == nil {
		log.Println("[consensus] No judge configured, assigning default scores")
		for _, c := range candidates {
			c.JudgeScore = 50 // Neutral default
		}
		return
	}

	// Build a comparison prompt for the judge
	var sb strings.Builder
	sb.WriteString("You are a senior code reviewer. Below are multiple code patches that all pass automated tests. ")
	sb.WriteString("Score each patch from 0 to 100 based on:\n")
	sb.WriteString("- Correctness and completeness (40 points)\n")
	sb.WriteString("- Code quality and readability (30 points)\n")
	sb.WriteString("- Edge case handling and robustness (30 points)\n\n")
	sb.WriteString("Return ONLY a JSON array of objects with 'patch_index' (0-based) and 'score' (integer 0-100).\n")
	sb.WriteString("Example: [{\"patch_index\": 0, \"score\": 85}, {\"patch_index\": 1, \"score\": 72}]\n\n")

	for i, c := range candidates {
		sb.WriteString(fmt.Sprintf("=== PATCH %d (%s/%s) ===\n```\n%s\n```\n\n", i, c.Patch.Provider, c.Patch.Model, c.Patch.Code))
	}

	judgeName := fmt.Sprintf("%s/%s", e.judge.Name(), e.judge.Model())
	e.emit(fmt.Sprintf("  🧑‍⚖️ Asking %s to evaluate %d patches...", judgeName, len(candidates)))

	result, err := e.judge.GeneratePatch(sb.String())
	if err != nil {
		log.Printf("[consensus] Judge failed: %v, using default scores", err)
		e.emit(fmt.Sprintf("  ⚠️ Judge failed: %v — using neutral scores", err))
		for _, c := range candidates {
			c.JudgeScore = 50
		}
		return
	}

	// Parse the judge's response
	scores := parseJudgeScores(result.Code, result.Explanation)
	for _, s := range scores {
		if s.Index >= 0 && s.Index < len(candidates) {
			candidates[s.Index].JudgeScore = float64(s.Score)
			e.emit(fmt.Sprintf("  📝 Patch %d (%s/%s): Judge score = %d/100",
				s.Index, candidates[s.Index].Patch.Provider, candidates[s.Index].Patch.Model, s.Score))
		}
	}

	// Fill in any unscored candidates
	for _, c := range candidates {
		if c.JudgeScore == 0 {
			c.JudgeScore = 50 // Default neutral
		}
	}
}

type judgeScore struct {
	Index int `json:"patch_index"`
	Score int `json:"score"`
}

func parseJudgeScores(code, explanation string) []judgeScore {
	// Try to parse the code field as JSON first, then the explanation
	for _, text := range []string{code, explanation} {
		// Try direct JSON parse
		var scores []judgeScore
		if err := json.Unmarshal([]byte(text), &scores); err == nil && len(scores) > 0 {
			return scores
		}

		// Try to extract JSON array from text
		re := regexp.MustCompile(`\[[\s\S]*?\]`)
		if m := re.FindString(text); m != "" {
			if err := json.Unmarshal([]byte(m), &scores); err == nil && len(scores) > 0 {
				return scores
			}
		}

		// Last resort: look for "patch_index": N, "score": N patterns
		patchRe := regexp.MustCompile(`"?patch_index"?\s*:\s*(\d+).*?"?score"?\s*:\s*(\d+)`)
		matches := patchRe.FindAllStringSubmatch(text, -1)
		if len(matches) > 0 {
			for _, m := range matches {
				idx, _ := strconv.Atoi(m[1])
				sc, _ := strconv.Atoi(m[2])
				scores = append(scores, judgeScore{Index: idx, Score: sc})
			}
			return scores
		}
	}
	return nil
}

// scoreSandboxPerformance gives a 0-100 score based on sandbox execution.
// Passing = minimum 70. Faster = higher score.
func scoreSandboxPerformance(result *sandbox.Result) float64 {
	if !result.Success {
		return 0
	}
	// Base 70 for passing. Bonus up to 30 based on speed.
	// Under 5s = full bonus, over 30s = no bonus
	bonus := 30.0
	ms := float64(result.DurationMs)
	if ms > 30000 {
		bonus = 0
	} else if ms > 5000 {
		bonus = 30.0 * (1.0 - (ms-5000)/25000)
	}
	return 70.0 + bonus
}

func filterActive(candidates []*Candidate) []*Candidate {
	var out []*Candidate
	for _, c := range candidates {
		if !c.Blocked {
			out = append(out, c)
		}
	}
	return out
}

func filterPassing(candidates []*Candidate) []*Candidate {
	var out []*Candidate
	for _, c := range candidates {
		if !c.Blocked && !c.Eliminated {
			out = append(out, c)
		}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// selfHeal implements iterative self-healing: when all patches fail sandbox,
// it feeds the error logs back to the LLMs and asks them to fix their patches.
func (e *Engine) selfHeal(failedCandidates []*Candidate, testScript string, report *Report, round int) *Report {
	if round > e.maxRetries {
		report.Summary = fmt.Sprintf("All patches failed after %d self-healing rounds", e.maxRetries)
		report.Candidates = failedCandidates
		return report
	}

	e.emit(fmt.Sprintf("🔄 **Self-Healing Round %d/%d** — Feeding errors back to LLMs...", round, e.maxRetries))

	// Collect error logs from failed candidates
	var errorFeedback strings.Builder
	errorFeedback.WriteString("Your previous code patch FAILED testing. Here are the errors:\n\n")
	for _, c := range failedCandidates {
		if c.SandboxResult != nil && !c.SandboxResult.Success {
			errorFeedback.WriteString(fmt.Sprintf("=== %s/%s (exit code %d) ===\n%s\n\n",
				c.Patch.Provider, c.Patch.Model, c.SandboxResult.ExitCode,
				truncate(c.SandboxResult.Logs, 2000)))
		}
	}
	errorFeedback.WriteString("\nPlease fix your code based on these errors. Return ONLY the corrected code in a markdown code block.")

	healPrompt := errorFeedback.String()

	// Re-query all solvers with the error feedback
	newPatches := llm.FanOut(e.solvers, healPrompt, e.onEvent)
	if len(newPatches) == 0 {
		report.Summary = fmt.Sprintf("Self-healing round %d produced no patches", round)
		report.Candidates = failedCandidates
		return report
	}

	// Build new candidates and run through safety + sandbox again
	newCandidates := make([]*Candidate, 0, len(newPatches))
	for _, p := range newPatches {
		c := &Candidate{Patch: p}
		c.SafetyResult = validation.ValidatePythonPatch(c.Patch.Code)
		if !c.SafetyResult.OK {
			c.Blocked = true
			continue
		}

		name := fmt.Sprintf("%s/%s", c.Patch.Provider, c.Patch.Model)
		e.emit(fmt.Sprintf("  🔄 Re-testing %s (healed)...", name))

		result, err := e.sandbox.RunVerification(c.Patch.Code, testScript)
		if err != nil {
			c.SandboxResult = &sandbox.Result{Success: false, Logs: err.Error()}
			c.Eliminated = true
		} else {
			c.SandboxResult = result
			if result.Success {
				c.SandboxScore = scoreSandboxPerformance(result)
				e.emit(fmt.Sprintf("  ✅ %s healed successfully! (score: %.1f)", name, c.SandboxScore))
			} else {
				c.Eliminated = true
				e.emit(fmt.Sprintf("  ❌ %s still failing after healing", name))
			}
		}
		newCandidates = append(newCandidates, c)
	}

	passing := filterPassing(newCandidates)
	if len(passing) == 0 {
		// Recurse for another round
		return e.selfHeal(newCandidates, testScript, report, round+1)
	}

	// Healed patches exist — continue with Phase 3 + 4
	e.emit("🎉 **Self-healing succeeded!** Continuing with consensus...")

	// Run structural similarity and judge on the healed patches
	// (reuse the same logic from Evaluate)
	fingerprints := map[string][]*Candidate{}
	for _, c := range passing {
		fp := validation.StructuralFingerprint(c.Patch.Code)
		fingerprints[fp] = append(fingerprints[fp], c)
	}
	maxCluster := 0
	for _, cluster := range fingerprints {
		if len(cluster) > maxCluster {
			maxCluster = len(cluster)
		}
	}
	for _, cluster := range fingerprints {
		score := 100.0
		if maxCluster > 1 {
			score = (float64(len(cluster)) / float64(maxCluster)) * 100.0
		}
		for _, c := range cluster {
			c.StructuralScore = score
		}
	}

	e.runJudgePhase(passing)

	for _, c := range passing {
		c.FinalScore = (c.SandboxScore * WeightSandbox) +
			(c.StructuralScore * WeightStructural) +
			(c.JudgeScore * WeightJudge)
	}

	sort.Slice(passing, func(i, j int) bool {
		return passing[i].FinalScore > passing[j].FinalScore
	})

	report.Winner = passing[0]
	report.PassedSandbox = len(passing)
	report.UniqueStructures = len(fingerprints)
	report.Candidates = append(failedCandidates, newCandidates...)
	report.Summary = fmt.Sprintf("=== RAVENMIND CONSENSUS (healed in round %d) ===\n🏆 Winner: %s/%s (score: %.1f)\n",
		round, report.Winner.Patch.Provider, report.Winner.Patch.Model, report.Winner.FinalScore)

	e.emit(fmt.Sprintf("🏆 **Winner (healed):** %s/%s with score %.1f",
		report.Winner.Patch.Provider, report.Winner.Patch.Model, report.Winner.FinalScore))
	return report
}

