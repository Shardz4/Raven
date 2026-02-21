"""
Raven Agent Module

Core components for the Raven autonomous bounty hunter:
- AgentCoordinator: Orchestrates the delegate → verify → monetize workflow
- CortensorLiveNetwork (cortensor_live.py): Real Cortensor Web2 API
- CortensorNetwork (cortensor_production.py): Production LLM network (OpenAI, Claude, etc.)
- DockerSandbox: Sandboxed execution
- X402Merchant: Payment gate
"""

from agent.coordinator import AgentCoordinator
from agent.cortensor_production import CortensorNetwork
from agent.cortensor_live import CortensorLiveNetwork
from agent.sandbox import DockerSandbox
from agent.x402 import X402Merchant

__all__ = [
    "AgentCoordinator",
    "CortensorNetwork",
    "CortensorLiveNetwork",
    "DockerSandbox",
    "X402Merchant",
]

__version__ = "0.1.0"

