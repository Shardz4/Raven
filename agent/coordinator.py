import os
import urllib.parse
from agent.llm_provider import LLMFactory
from agent.sandbox import DockerSandbox
from agent.validation import validate_patch_code

class AgentCoordinator:
    def __init__(self):
        self.sandbox = DockerSandbox()
        # Ensure Docker is ready
        self.sandbox.build_image()
        # Initialize providers
        self.providers = LLMFactory.create_all()

    def solve_issue(self, issue_url):
        try:
            yield "event", "🔍 **Analyzing Issue:** " + issue_url

            active_providers = list(self.providers.values())
            if not active_providers:
                yield "error", "❌ No LLM providers configured. Set OPENAI_API_KEY, ANTHROPIC_API_KEY, etc. in .env"
                return
            
            redundancy = _int_env("RAVEN_REDUNDANCY", min(3, len(active_providers)))
            selected_providers = active_providers[:redundancy]
            
            yield "event", f"📡 **Engaging AI Developers:** Requesting solutions from {len(selected_providers)} LLMs..."

            candidates = []
            for provider in selected_providers:
                try:
                    yield "event", f"⚡ Prompting {provider.model} ({provider.__class__.__name__})..."
                    patch = provider.generate_patch(issue_url)
                    patch["model"] = provider.model
                    candidates.append(patch)
                    yield "event", f"📦 Received patch from **{provider.model}**"
                except Exception as e:
                    yield "error", f"⚠️ Error requesting from {provider.model}: {e}"

            if not candidates:
                yield "error", "❌ No valid solutions received from any AI provider."
                return

            # Dynamic Execution: Parse repo URL & Create Sandbox execution script
            parsed_url = urllib.parse.urlsplit(issue_url)
            path_parts = [p for p in parsed_url.path.split('/') if p]

            if len(path_parts) >= 2 and parsed_url.netloc == "github.com":
                org, repo = path_parts[0], path_parts[1]
                target_repo_url = f"https://github.com/{org}/{repo}.git"
            else:
                # Fallback for arbitrary or invalid URLs
                target_repo_url = "https://github.com/demo-organization/demo-repo.git"

            run_tests_sh = f"""#!/bin/bash
set -e
echo "Cloning target repository: {target_repo_url}"
git clone --depth 1 {target_repo_url} target_repo || exit 1
cd target_repo
echo "Applying AI generated patch (solution.py)..."
cp /app/solution.py .
echo "Installing dependencies and running tests..."
if [ -f requirements.txt ]; then pip install -r requirements.txt; fi
python -m pytest -q
"""

            logs = []
            passing = []
            validation_blocked = 0

            for cand in candidates:
                try:
                    model_id = cand.get("model", "Unknown")
                    code = cand.get("code", "")

                    v = validate_patch_code(code)
                    if not v.ok:
                        validation_blocked += 1
                        logs.append(f"Model: {model_id}\nResult: BLOCKED\nReason: {v.reason}\n---")
                        yield "event", f"⛔ Patch from **{model_id}** blocked: {v.reason}"
                        continue

                    yield "event", f"Testing Patch from **{model_id}** in Sandbox..."
                    result = self.sandbox.run_verification(code, run_tests_sh)

                    logs.append(f"Model: {model_id}\nResult: {'PASS' if result['success'] else 'FAIL'}\nLogs: {result['logs']}\n---")

                    if result['success']:
                        passing.append(cand)
                except Exception as e:
                    yield "error", f"⚠️ Error testing {cand.get('model', 'Unknown')}: {str(e)}"
                    continue

            if not passing:
                yield "error", "❌ All AI generated patches failed sandbox verification."
                return

            # Consensus (AI Oracle-style): pick most-agreed passing patch by normalized code.
            winner = _select_consensus_winner(passing)
            consensus_note = _consensus_report(passing, winner)
            if validation_blocked:
                consensus_note = f"{consensus_note}\n\nBlocked by validation: {validation_blocked}"
            logs.append("\n=== CONSENSUS ===\n" + consensus_note + "\n")

            yield "event", f"🏆 **Winner Found:** {winner.get('model', 'Unknown')}. Extracting code patch..."

            final_bundle = {
                "winner": winner.get("model", "Unknown"),
                "verification_logs": "\n".join(logs),
                "code": winner.get("code", ""),
                "explanation": winner.get("explanation", "")
            }

            yield "complete", final_bundle
        
        except Exception as e:
            yield "error", f"❌ Unexpected error in workflow: {str(e)}"

def _normalize_code(code: str) -> str:
    return "\n".join(line.rstrip() for line in (code or "").strip().splitlines()).strip()

def _select_consensus_winner(passing_candidates):
    buckets = {}
    order = []
    for cand in passing_candidates:
        code = _normalize_code(cand.get("code", ""))
        if code not in buckets:
            buckets[code] = []
            order.append(code)
        buckets[code].append(cand)

    best_code = None
    best_count = -1
    for code in order:
        count = len(buckets[code])
        if count > best_count:
            best_count = count
            best_code = code

    return buckets[best_code][0]

def _consensus_report(passing_candidates, winner):
    buckets = {}
    for cand in passing_candidates:
        code = _normalize_code(cand.get("code", ""))
        buckets.setdefault(code, []).append(cand.get("model", "Unknown"))

    lines = []
    lines.append(f"Passing patches: {len(passing_candidates)}")
    lines.append(f"Unique passing solutions: {len(buckets)}")
    for i, (code, miners) in enumerate(sorted(buckets.items(), key=lambda kv: len(kv[1]), reverse=True), start=1):
        preview = (code.splitlines()[0] if code else "").strip()
        lines.append(f"{i}. Votes={len(miners)} Models={miners} Preview={preview[:80]}")
    lines.append(f"Selected winner: {winner.get('model', 'Unknown')}")
    return "\n".join(lines)

def _int_env(name: str, default: int) -> int:
    try:
        v = os.getenv(name)
        return int(v) if v is not None else default
    except ValueError:
        return default
