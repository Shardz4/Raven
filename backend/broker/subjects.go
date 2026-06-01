package broker

const (
	// SubjectJobs is where new job requests are published.
	SubjectJobs = "raven.jobs"

	// SubjectSolverPrefix is the prefix for solver models.
	SubjectSolverPrefix = "raven.solver."

	// SubjectPatches is where raw generated patches from solvers are published.
	SubjectPatches = "raven.patches"

	// SubjectPatchesSafe is where safe patches from validation are published.
	SubjectPatchesSafe = "raven.patches.safe"

	// SubjectPatchesBlocked is where blocked patches from validation are published.
	SubjectPatchesBlocked = "raven.patches.blocked"

	// SubjectSandboxResults is where sandbox run results are published.
	SubjectSandboxResults = "raven.sandbox.results"

	// SubjectConsensusWinner is where final consensus results/winners are published.
	SubjectConsensusWinner = "raven.consensus.winner"

	// SubjectEventsPrefix is used to stream events to SSE clients.
	SubjectEventsPrefix = "raven.events."
)
