import os
from agent.cortensor_production import CortensorNetwork as ProductionCortensorNetwork
from agent.cortensor_live import CortensorLiveNetwork
from agent.sandbox import DockerSandbox
from agent.x402 import X402Merchant
from agent.validation import validate_patch_code
import time

class AgentCoordinator:
    def __init__(self):
        raw_mode = (os.getenv("RAVEN_MINER_MODE") or "").strip().lower()
        mode = raw_mode
        if not mode:
            # Default to Cortensor live network
            mode = "cortensor"

        if mode == "cortensor":
            self.network = CortensorLiveNetwork()
        elif mode == "production":
            self.network = ProductionCortensorNetwork()
        else:
            # Fallback to Cortensor if unknown mode specified
            self.network = CortensorLiveNetwork()
        self.sandbox = DockerSandbox()
        self.merchant = X402Merchant()

        # Ensure Docker is ready
        self.sandbox.build_image()

    def solve_issue(self, issue_url):
        try:
            yield "event", "🔍 **Analyzing Issue:** " + issue_url

            # 1. DELEGATE
            redundancy = _int_env("RAVEN_REDUNDANCY", 3)
            yield "event", f"📡 **Delegating to Miner Network:** Requesting {redundancy} redundant solutions..."

            candidates = []
            if hasattr(self.network, "request_patches_stream_events"):
                for kind, payload in self.network.request_patches_stream_events(issue_url, redundancy=redundancy):
                    if kind == "event":
                        yield "event", str(payload)
                    elif kind == "candidate":
                        cand = payload
                        candidates.append(cand)
                        yield "event", f"📦 Patch received from **{cand.get('miner_id', 'Unknown')}**"
            elif hasattr(self.network, "request_patches_stream"):
                for cand in self.network.request_patches_stream(issue_url, redundancy=redundancy):
                    candidates.append(cand)
                    yield "event", f"📦 Patch received from **{cand.get('miner_id', 'Unknown')}**"
            else:
                candidates = self.network.request_patches(issue_url, redundancy=redundancy)
            
            if not candidates:
                yield "error", "❌ No solutions received from miner network."
                return

            # Dynamic Execution: Parse repo URL & Create Sandbox execution script
            import urllib.parse
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
                    miner_id = cand.get("miner_id", "Unknown")
                    code = cand.get("code", "")

                    v = validate_patch_code(code)
                    if not v.ok:
                        validation_blocked += 1
                        logs.append(f"Miner: {miner_id}\\nResult: BLOCKED\\nReason: {v.reason}\\n---")
                        yield "event", f"⛔ Patch from **{miner_id}** blocked: {v.reason}"
                        continue

                    yield "event", f"Testing Patch from **{miner_id}** in Sandbox..."
                    result = self.sandbox.run_verification(code, run_tests_sh)

                    logs.append(f"Miner: {miner_id}\nResult: {'PASS' if result['success'] else 'FAIL'}\nLogs: {result['logs']}\n---")

                    if result['success']:
                        passing.append(cand)
                except Exception as e:
                    yield "error", f"⚠️ Error testing {cand.get('miner_id', 'Unknown')}: {str(e)}"
                    continue

            if not passing:
                yield "error", "❌ No consensus reached. All patches failed verification."
                return

            # 2.5 CONSENSUS (AI Oracle-style): pick most-agreed passing patch by normalized code.
            winner = _select_consensus_winner(passing)
            consensus_note = _consensus_report(passing, winner)
            if validation_blocked:
                consensus_note = f"{consensus_note}\n\nBlocked by validation: {validation_blocked}"
            logs.append("\n=== CONSENSUS ===\n" + consensus_note + "\n")

            # 3. MONETIZE (x402)
            yield "event", f"🏆 **Winner Found:** {winner.get('miner_id', 'Unknown')}. Creating x402 Lock..."

            lock_data = self.merchant.create_locked_content(winner.get("code", ""))

            final_bundle = {
                "winner": winner.get("miner_id", "Unknown"),
                "verification_logs": "\n".join(logs),
                "payment_link": lock_data['payment_link'],
                "invoice_id": lock_data['invoice_id']
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

    # Pick bucket with most votes; tie-breaker: first seen.
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
        buckets.setdefault(code, []).append(cand.get("miner_id", "Unknown"))

    lines = []
    lines.append(f"Passing patches: {len(passing_candidates)}")
    lines.append(f"Unique passing solutions: {len(buckets)}")
    for i, (code, miners) in enumerate(sorted(buckets.items(), key=lambda kv: len(kv[1]), reverse=True), start=1):
        preview = (code.splitlines()[0] if code else "").strip()
        lines.append(f"{i}. Votes={len(miners)} Miners={miners} Preview={preview[:80]}")
    lines.append(f"Selected winner: {winner.get('miner_id', 'Unknown')}")
    return "\n".join(lines)


def _int_env(name: str, default: int) -> int:
    try:
        v = os.getenv(name)
        return int(v) if v is not None else default
    except ValueError:
        return default
