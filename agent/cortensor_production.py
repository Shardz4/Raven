"""
PRODUCTION-READY LLM NETWORK
Uses real AI models (OpenAI, Claude, DeepSeek, Grok, Ollama)
"""

import random
import logging
import time
import os
from typing import List, Dict, Optional
from agent.llm_provider import LLMFactory

logger = logging.getLogger(__name__)


class CortensorNetwork:
    """
    Production-ready miner network using real LLM APIs
    """

    def __init__(self, providers: Optional[List[str]] = None):
        """
        Initialize Cortensor Network

        Args:
            providers: List of provider names ["openai", "claude", "ollama", "deepseek", "grok"]
                      If None, auto-detects available providers
        """
        self.miners = [
            {
                "id": "Miner_Alpha",
                "provider": "openai",
                "model": "gpt-4-turbo-preview",
                "priority": 1,  # High quality
            },
            {
                "id": "Miner_Beta",
                "provider": "claude",
                "model": "claude-3-opus-20240229",
                "priority": 1,
            },
            {
                "id": "Miner_Gamma",
                "provider": "openai",
                "model": "gpt-3.5-turbo",
                "priority": 3,  # Cheaper fallback
            },
            {
                "id": "Miner_Delta",
                "provider": "ollama",
                "model": "llama2",
                "priority": 4,  # Local, free but slower
            },
            {
                "id": "Miner_Epsilon",
                "provider": "deepseek",
                "model": "deepseek-chat",
                "priority": 2,
            },
            {
                "id": "Miner_Zeta",
                "provider": "grok",
                "model": "grok-beta",
                "priority": 2,
            },
        ]

        # Provider allowlist (optional) + per-(provider, model) cache
        self._requested_providers = [p.lower() for p in providers] if providers else None
        self._provider_cache: Dict[str, object] = {}
        self._available_provider_names = self._detect_available_providers()

        if not self._available_provider_names:
            raise RuntimeError(
                "No LLM providers available. Set at least one: OPENAI_API_KEY, ANTHROPIC_API_KEY, "
                "DEEPSEEK_API_KEY, XAI_API_KEY, or ensure Ollama is running."
            )
        logger.info("✓ LLM providers available: %s", ", ".join(sorted(self._available_provider_names)))

    def _detect_available_providers(self) -> List[str]:
        names = []
        candidates = ["openai", "claude", "deepseek", "grok", "ollama"]
        if self._requested_providers is not None:
            candidates = [c for c in candidates if c in self._requested_providers]

        for name in candidates:
            if name == "openai" and os.getenv("OPENAI_API_KEY"):
                names.append("openai")
            elif name == "claude" and os.getenv("ANTHROPIC_API_KEY"):
                names.append("claude")
            elif name == "deepseek" and os.getenv("DEEPSEEK_API_KEY"):
                names.append("deepseek")
            elif name == "grok" and (os.getenv("XAI_API_KEY") or os.getenv("GROK_API_KEY")):
                names.append("grok")
            elif name == "ollama":
                # Ollama is detected lazily; only include if reachable at runtime.
                names.append("ollama")

        return names

    def _get_provider(self, provider_name: str, model: str):
        key = f"{provider_name}:{model}"
        if key in self._provider_cache:
            return self._provider_cache[key]

        provider = LLMFactory.create(provider_name, model=model)
        self._provider_cache[key] = provider
        return provider

    def request_patches(
        self,
        issue_description: str,
        issue_code: Optional[str] = None,
        redundancy: int = 3
    ) -> List[Dict]:
        """
        Request code patches from multiple miners

        Args:
            issue_description: GitHub issue description/title
            issue_code: Current code (if available)
            redundancy: Number of different miners to query

        Returns:
            List of patches with metadata

        Raises:
            RuntimeError: If no patches could be generated
        """
        # Build context
        context = issue_description
        if issue_code:
            context += f"\n\nCurrent Code:\n{issue_code}"

        # Select miners
        available_miners = [m for m in self.miners if m["provider"] in self._available_provider_names]
        if not available_miners:
            raise RuntimeError("No LLM providers available (check env keys)")

        # Limit redundancy
        redundancy = min(redundancy, len(available_miners))
        selected_miners = random.sample(available_miners, redundancy)

        logger.info(f"📡 Broadcasting to {len(selected_miners)} LLM miners...")

        results = []
        total_cost = 0.0

        for miner in selected_miners:
            try:
                logger.info(f"Requesting patch from {miner['id']} ({miner['provider']}/{miner['model']})...")
                provider = self._get_provider(miner["provider"], miner["model"])

                # Generate patch (REAL API CALL or local)
                start_time = time.time()
                patch_result = provider.generate_patch(context)
                duration = time.time() - start_time

                total_cost += patch_result.get('cost', 0)

                # Build result
                result = {
                    "miner_id": miner['id'],
                    "model": miner['model'],
                    "provider": miner['provider'],
                    "priority": miner['priority'],
                    "code": patch_result['code'],
                    "explanation": patch_result['explanation'],
                    "signature": patch_result['signature'],
                    "tokens": patch_result['tokens'],
                    "cost": patch_result.get('cost', 0),
                    "duration": f"{duration:.2f}s",
                    "timestamp": time.time()
                }

                results.append(result)
                logger.info(f"✓ {miner['id']}: {result['tokens'].get('total', 0)} tokens, ${result['cost']:.4f}")

            except Exception as e:
                logger.error(f"✗ {miner['id']}: {str(e)}")
                continue

        if not results:
            raise RuntimeError("No miners returned valid patches")

        logger.info(f"📊 Total cost: ${total_cost:.4f}")
        return results

    def request_patches_stream(
        self,
        issue_description: str,
        issue_code: Optional[str] = None,
        redundancy: int = 3,
    ):
        """Generator version of request_patches for progressive UI updates."""
        context = issue_description
        if issue_code:
            context += f"\n\nCurrent Code:\n{issue_code}"

        available_miners = [m for m in self.miners if m["provider"] in self._available_provider_names]
        if not available_miners:
            raise RuntimeError("No LLM providers available (check env keys)")

        redundancy = min(redundancy, len(available_miners))
        selected_miners = random.sample(available_miners, redundancy)

        logger.info(f"📡 Broadcasting to {len(selected_miners)} LLM miners...")

        total_cost = 0.0
        yielded = 0

        for miner in selected_miners:
            try:
                logger.info(f"Requesting patch from {miner['id']} ({miner['provider']}/{miner['model']})...")
                provider = self._get_provider(miner["provider"], miner["model"])

                start_time = time.time()
                patch_result = provider.generate_patch(context)
                duration = time.time() - start_time

                total_cost += patch_result.get("cost", 0)

                result = {
                    "miner_id": miner["id"],
                    "model": miner["model"],
                    "provider": miner["provider"],
                    "priority": miner["priority"],
                    "code": patch_result["code"],
                    "explanation": patch_result.get("explanation", ""),
                    "signature": patch_result.get("signature", ""),
                    "tokens": patch_result.get("tokens", {"total": 0}),
                    "cost": patch_result.get("cost", 0),
                    "duration": f"{duration:.2f}s",
                    "timestamp": time.time(),
                }

                yielded += 1
                yield result
                logger.info(f"✓ {miner['id']}: {result['tokens'].get('total', 0)} tokens, ${result['cost']:.4f}")
            except Exception as e:
                logger.error(f"✗ {miner['id']}: {str(e)}")
                continue

        if not yielded:
            raise RuntimeError("No miners returned valid patches")

        logger.info(f"📊 Total cost: ${total_cost:.4f}")

    def get_network_status(self) -> Dict:
        """Get current network status"""
        return {
            "total_miners": len(self.miners),
            "available_providers": len(self._available_provider_names),
            "status": "operational",
            "providers": sorted(self._available_provider_names),
        }


# ============================================================================
# EXAMPLE USAGE
# ============================================================================

if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)

    # Example: Create network and request patches
    print("\n=== Cortensor Network Example ===\n")

    try:
        network = CortensorNetwork()
        print(f"Network Status: {network.get_network_status()}\n")

        # Request patches
        patches = network.request_patches(
            issue_description="Create a function to reverse a string in Python",
            redundancy=2
        )

        print(f"\n📦 Received {len(patches)} patches:\n")
        for patch in patches:
            print(f"  Miner: {patch['miner_id']} ({patch['provider']})")
            print(f"  Model: {patch['model']}")
            print(f"  Tokens: {patch['tokens']}")
            print(f"  Cost: ${patch['cost']:.4f}")
            print(f"  Duration: {patch['duration']}")
            print(f"  Code snippet: {patch['code'][:100]}...\n")

    except Exception as e:
        print(f"Error: {e}")
