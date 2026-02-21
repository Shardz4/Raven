#!/usr/bin/env python3
"""
PRODUCTION-READY LLM PROVIDER
Complete implementation for OpenAI, Claude, and local models

Install: pip install openai anthropic requests python-dotenv
"""

import os
import logging
import hashlib
import hmac
from abc import ABC, abstractmethod
from typing import Dict, List, Optional
from dotenv import load_dotenv
import time

load_dotenv()
logger = logging.getLogger(__name__)

# ============================================================================
# BASE PROVIDER CLASS
# ============================================================================

class LLMProvider(ABC):
    """Abstract base for all LLM providers"""

    @abstractmethod
    def generate_patch(self, issue_description: str) -> Dict:
        """Generate code patch for issue"""
        pass

    @abstractmethod
    def get_cost_per_1k_tokens(self) -> float:
        """Return approximate cost per 1000 tokens"""
        pass

    @staticmethod
    def sign_patch(code: str, secret: str = None) -> str:
        """Cryptographically sign generated code"""
        if secret is None:
            secret = os.getenv("MINER_SECRET_KEY", "default-secret")

        code_hash = hashlib.sha256(code.encode()).digest()
        signature = hmac.new(
            secret.encode(),
            code_hash,
            hashlib.sha256
        ).hexdigest()

        return signature


# ============================================================================
# OPENAI PROVIDER
# ============================================================================

class OpenAIProvider(LLMProvider):
    """OpenAI GPT-4/GPT-3.5 provider"""

    def __init__(self, model: str = "gpt-4-turbo-preview", temperature: float = 0.3):
        """
        Initialize OpenAI provider

        Args:
            model: gpt-4, gpt-4-turbo-preview, gpt-3.5-turbo
            temperature: 0.0-1.0 (lower = deterministic, higher = creative)
        """
        try:
            import openai
            self.openai = openai
        except ImportError:
            raise ImportError("pip install openai")

        api_key = os.getenv("OPENAI_API_KEY")
        if not api_key:
            raise ValueError("OPENAI_API_KEY not set in environment")

        self.openai.api_key = api_key
        self.model = model
        self.temperature = temperature

        logger.info(f"✓ OpenAI Provider initialized: {model}")

    def generate_patch(self, issue_description: str) -> Dict:
        """Generate patch using OpenAI API"""
        try:
            logger.info(f"Requesting patch from OpenAI ({self.model})...")

            response = self.openai.ChatCompletion.create(
                model=self.model,
                messages=[
                    {
                        "role": "system",
                        "content": """You are an expert Python programmer.
Generate high-quality code fixes that:
- Are production-ready
- Include proper error handling
- Have clear comments
- Follow Python best practices
- Are thoroughly tested"""
                    },
                    {
                        "role": "user",
                        "content": f"""GitHub Issue:\n{issue_description}

Please provide a Python fix for this issue.
Return ONLY valid Python code in a markdown code block."""
                    }
                ],
                temperature=self.temperature,
                max_tokens=2000,
                timeout=30
            )

            content = response['choices'][0]['message']['content']
            code = self._extract_code(content)

            return {
                "code": code,
                "explanation": content,
                "model": self.model,
                "provider": "openai",
                "tokens": {
                    "prompt": response['usage']['prompt_tokens'],
                    "completion": response['usage']['completion_tokens'],
                    "total": response['usage']['total_tokens']
                },
                "cost": self._calculate_cost(response['usage']),
                "signature": self.sign_patch(code)
            }

        except Exception as e:
            logger.error(f"OpenAI error: {e}")
            raise

    def _extract_code(self, text: str) -> str:
        """Extract Python code from response"""
        import re

        # Try markdown code block
        match = re.search(r'```(?:python)?\n(.*?)\n```', text, re.DOTALL)
        if match:
            return match.group(1).strip()

        # Try inline code
        match = re.search(r'`([^`]+)`', text)
        if match:
            return match.group(1).strip()

        return text

    def _calculate_cost(self, usage) -> float:
        """Calculate cost of API call"""
        # gpt-4-turbo: $0.01/$0.03
        # gpt-4: $0.03/$0.06
        # gpt-3.5: $0.0005/$0.0015

        if "gpt-4-turbo" in self.model:
            input_cost = usage['prompt_tokens'] * 0.01 / 1000
            output_cost = usage['completion_tokens'] * 0.03 / 1000
        elif "gpt-4" in self.model:
            input_cost = usage['prompt_tokens'] * 0.03 / 1000
            output_cost = usage['completion_tokens'] * 0.06 / 1000
        else:  # gpt-3.5
            input_cost = usage['prompt_tokens'] * 0.0005 / 1000
            output_cost = usage['completion_tokens'] * 0.0015 / 1000

        return input_cost + output_cost

    def get_cost_per_1k_tokens(self) -> float:
        """Return approx cost per 1000 tokens"""
        if "gpt-4-turbo" in self.model:
            return 0.015  # averaged input+output
        elif "gpt-4" in self.model:
            return 0.045
        else:
            return 0.001


# ============================================================================
# ANTHROPIC CLAUDE PROVIDER
# ============================================================================

class ClaudeProvider(LLMProvider):
    """Anthropic Claude provider"""

    def __init__(self, model: str = "claude-3-opus-20240229", temperature: float = 0.3):
        """
        Initialize Claude provider

        Args:
            model: claude-3-opus, claude-3-sonnet, claude-3-haiku
            temperature: 0.0-1.0
        """
        try:
            import anthropic
            self.anthropic = anthropic
        except ImportError:
            raise ImportError("pip install anthropic")

        api_key = os.getenv("ANTHROPIC_API_KEY")
        if not api_key:
            raise ValueError("ANTHROPIC_API_KEY not set in environment")

        self.client = self.anthropic.Anthropic(api_key=api_key)
        self.model = model
        self.temperature = temperature

        logger.info(f"✓ Claude Provider initialized: {model}")

    def generate_patch(self, issue_description: str) -> Dict:
        """Generate patch using Claude API"""
        try:
            logger.info(f"Requesting patch from Claude ({self.model})...")

            message = self.client.messages.create(
                model=self.model,
                max_tokens=2000,
                temperature=self.temperature,
                messages=[
                    {
                        "role": "user",
                        "content": f"""GitHub Issue:\n{issue_description}

Please provide a Python fix for this issue.
Return ONLY valid Python code in a markdown code block.

Requirements:
- Production-ready code
- Proper error handling
- Clear comments
- Best practices"""
                    }
                ]
            )

            content = message.content[0].text
            code = self._extract_code(content)

            return {
                "code": code,
                "explanation": content,
                "model": self.model,
                "provider": "anthropic",
                "tokens": {
                    "input": message.usage.input_tokens,
                    "output": message.usage.output_tokens,
                    "total": message.usage.input_tokens + message.usage.output_tokens
                },
                "cost": self._calculate_cost(message.usage),
                "signature": self.sign_patch(code)
            }

        except Exception as e:
            logger.error(f"Claude error: {e}")
            raise

    def _extract_code(self, text: str) -> str:
        """Extract Python code from response"""
        import re

        match = re.search(r'```(?:python)?\n(.*?)\n```', text, re.DOTALL)
        if match:
            return match.group(1).strip()

        match = re.search(r'`([^`]+)`', text)
        if match:
            return match.group(1).strip()

        return text

    def _calculate_cost(self, usage) -> float:
        """Calculate cost of API call"""
        # claude-3-opus: $0.015/$0.075
        # claude-3-sonnet: $0.003/$0.015
        # claude-3-haiku: $0.00025/$0.00125

        if "opus" in self.model:
            input_cost = usage.input_tokens * 0.015 / 1000
            output_cost = usage.output_tokens * 0.075 / 1000
        elif "sonnet" in self.model:
            input_cost = usage.input_tokens * 0.003 / 1000
            output_cost = usage.output_tokens * 0.015 / 1000
        else:  # haiku
            input_cost = usage.input_tokens * 0.00025 / 1000
            output_cost = usage.output_tokens * 0.00125 / 1000

        return input_cost + output_cost

    def get_cost_per_1k_tokens(self) -> float:
        """Return approx cost per 1000 tokens"""
        if "opus" in self.model:
            return 0.045
        elif "sonnet" in self.model:
            return 0.009
        else:  # haiku
            return 0.000375


# ============================================================================
# LOCAL OLLAMA PROVIDER (Self-hosted)
# ============================================================================

class OllamaProvider(LLMProvider):
    """Local Ollama provider (self-hosted, zero API cost)"""

    def __init__(self, model: str = "llama2", base_url: str = "http://localhost:11434"):
        """
        Initialize Ollama provider

        Args:
            model: llama2, mistral, neural-chat, etc.
            base_url: Ollama server URL
        """
        import requests
        self.requests = requests

        self.model = model
        self.base_url = base_url.rstrip('/')

        # Test connection
        try:
            response = self.requests.get(f"{self.base_url}/api/tags", timeout=5)
            response.raise_for_status()
            logger.info(f"✓ Ollama Provider initialized: {model}")
        except Exception as e:
            raise RuntimeError(f"Ollama server not accessible at {self.base_url}: {e}")

    def generate_patch(self, issue_description: str) -> Dict:
        """Generate patch using local Ollama model"""
        try:
            logger.info(f"Requesting patch from Ollama ({self.model})...")

            prompt = f"""GitHub Issue:
{issue_description}

Please provide a Python fix for this issue.
Return ONLY valid Python code."""

            response = self.requests.post(
                f"{self.base_url}/api/generate",
                json={
                    "model": self.model,
                    "prompt": prompt,
                    "stream": False
                },
                timeout=120  # Ollama can be slow
            )

            response.raise_for_status()
            result = response.json()

            code = self._extract_code(result['response'])

            return {
                "code": code,
                "explanation": result['response'],
                "model": self.model,
                "provider": "ollama",
                "tokens": {
                    "total": result.get('eval_count', 0)
                },
                "cost": 0.0,  # Self-hosted = zero API cost
                "signature": self.sign_patch(code)
            }

        except Exception as e:
            logger.error(f"Ollama error: {e}")
            raise

    def _extract_code(self, text: str) -> str:
        """Extract Python code from response"""
        import re

        match = re.search(r'```(?:python)?\n(.*?)\n```', text, re.DOTALL)
        if match:
            return match.group(1).strip()

        return text

    def get_cost_per_1k_tokens(self) -> float:
        """Return cost per 1000 tokens"""
        return 0.0  # Self-hosted


# ============================================================================
# DEEPSEEK PROVIDER (HTTP, OpenAI-style)
# ============================================================================


class DeepSeekProvider(LLMProvider):
    """DeepSeek provider via HTTP API (OpenAI-style schema)"""

    def __init__(self, model: str = "deepseek-chat", base_url: Optional[str] = None, temperature: float = 0.3):
        import requests
        self.requests = requests

        api_key = os.getenv("DEEPSEEK_API_KEY")
        if not api_key:
            raise ValueError("DEEPSEEK_API_KEY not set in environment")

        self.api_key = api_key
        self.model = model
        self.temperature = temperature
        self.base_url = (base_url or os.getenv("DEEPSEEK_BASE_URL") or "https://api.deepseek.com").rstrip("/")

        logger.info(f"✓ DeepSeek Provider initialized: {model}")

    def generate_patch(self, issue_description: str) -> Dict:
        """Generate patch using DeepSeek API"""
        prompt = f"""GitHub Issue:
{issue_description}

Please provide a Python fix for this issue.
Return ONLY valid Python code in a markdown code block."""

        try:
            logger.info(f"Requesting patch from DeepSeek ({self.model})...")
            resp = self.requests.post(
                f"{self.base_url}/v1/chat/completions",
                headers={"Authorization": f"Bearer {self.api_key}", "Content-Type": "application/json"},
                json={
                    "model": self.model,
                    "temperature": self.temperature,
                    "messages": [
                        {"role": "system", "content": "You are an expert Python programmer."},
                        {"role": "user", "content": prompt},
                    ],
                },
                timeout=30,
            )
            resp.raise_for_status()
            data = resp.json()
            content = data["choices"][0]["message"]["content"]
            code = OpenAIProvider._extract_code(self, content)

            usage = data.get("usage", {}) or {}
            return {
                "code": code,
                "explanation": content,
                "model": self.model,
                "provider": "deepseek",
                "tokens": {
                    "prompt": usage.get("prompt_tokens", 0),
                    "completion": usage.get("completion_tokens", 0),
                    "total": usage.get("total_tokens", 0),
                },
                "cost": 0.0,
                "signature": self.sign_patch(code),
            }
        except Exception as e:
            logger.error(f"DeepSeek error: {e}")
            raise

    def get_cost_per_1k_tokens(self) -> float:
        """Return approx cost per 1000 tokens (not priced here)"""
        return 0.0


# ============================================================================
# GROK (xAI) PROVIDER (HTTP, OpenAI-style)
# ============================================================================


class GrokProvider(LLMProvider):
    """xAI Grok provider via HTTP API"""

    def __init__(self, model: str = "grok-beta", base_url: Optional[str] = None, temperature: float = 0.3):
        import requests
        self.requests = requests

        api_key = os.getenv("XAI_API_KEY") or os.getenv("GROK_API_KEY")
        if not api_key:
            raise ValueError("XAI_API_KEY or GROK_API_KEY not set in environment")

        self.api_key = api_key
        self.model = model
        self.temperature = temperature
        self.base_url = (base_url or os.getenv("XAI_BASE_URL") or "https://api.x.ai").rstrip("/")

        logger.info(f"✓ Grok Provider initialized: {model}")

    def generate_patch(self, issue_description: str) -> Dict:
        """Generate patch using Grok (xAI) API"""
        prompt = f"""GitHub Issue:
{issue_description}

Please provide a Python fix for this issue.
Return ONLY valid Python code in a markdown code block."""

        try:
            logger.info(f"Requesting patch from Grok ({self.model})...")
            resp = self.requests.post(
                f"{self.base_url}/v1/chat/completions",
                headers={"Authorization": f"Bearer {self.api_key}", "Content-Type": "application/json"},
                json={
                    "model": self.model,
                    "temperature": self.temperature,
                    "messages": [
                        {"role": "system", "content": "You are an expert Python programmer."},
                        {"role": "user", "content": prompt},
                    ],
                },
                timeout=30,
            )
            resp.raise_for_status()
            data = resp.json()
            content = data["choices"][0]["message"]["content"]
            code = OpenAIProvider._extract_code(self, content)

            usage = data.get("usage", {}) or {}
            return {
                "code": code,
                "explanation": content,
                "model": self.model,
                "provider": "grok",
                "tokens": {
                    "prompt": usage.get("prompt_tokens", 0),
                    "completion": usage.get("completion_tokens", 0),
                    "total": usage.get("total_tokens", 0),
                },
                "cost": 0.0,
                "signature": self.sign_patch(code),
            }
        except Exception as e:
            logger.error(f"Grok error: {e}")
            raise

    def get_cost_per_1k_tokens(self) -> float:
        """Return approx cost per 1000 tokens (not priced here)"""
        return 0.0


# ============================================================================
# PROVIDER FACTORY
# ============================================================================

class LLMFactory:
    """Factory for creating LLM providers"""

    @staticmethod
    def create(provider_name: str, **kwargs) -> LLMProvider:
        """
        Create LLM provider

        Args:
            provider_name: "openai", "claude", "ollama", "deepseek", "grok"
            **kwargs: Provider-specific arguments

        Returns:
            LLMProvider instance
        """
        name = provider_name.lower()
        if name == "openai":
            return OpenAIProvider(**kwargs)
        elif name == "claude":
            return ClaudeProvider(**kwargs)
        elif name == "ollama":
            return OllamaProvider(**kwargs)
        elif name == "deepseek":
            return DeepSeekProvider(**kwargs)
        elif name in ("grok", "xai"):
            return GrokProvider(**kwargs)
        else:
            raise ValueError(f"Unknown provider: {provider_name}")

    @staticmethod
    def create_all() -> Dict[str, LLMProvider]:
        """Create all available providers"""
        providers = {}

        # OpenAI (if available)
        if os.getenv("OPENAI_API_KEY"):
            try:
                providers["openai_gpt4"] = OpenAIProvider(model="gpt-4-turbo-preview")
                providers["openai_gpt35"] = OpenAIProvider(model="gpt-3.5-turbo")
            except Exception as e:
                logger.warning(f"Could not init OpenAI: {e}")

        # Claude (if available)
        if os.getenv("ANTHROPIC_API_KEY"):
            try:
                providers["claude_opus"] = ClaudeProvider(model="claude-3-opus-20240229")
                providers["claude_haiku"] = ClaudeProvider(model="claude-3-haiku-20240307")
            except Exception as e:
                logger.warning(f"Could not init Claude: {e}")

        # DeepSeek (if available)
        if os.getenv("DEEPSEEK_API_KEY") or os.getenv("DEEPSEEK_BASE_URL"):
            try:
                providers["deepseek"] = DeepSeekProvider()
            except Exception as e:
                logger.warning(f"Could not init DeepSeek: {e}")

        # Grok / xAI (if available)
        if os.getenv("XAI_API_KEY") or os.getenv("GROK_API_KEY"):
            try:
                providers["grok"] = GrokProvider()
            except Exception as e:
                logger.warning(f"Could not init Grok: {e}")

        # Ollama (if available)
        try:
            providers["ollama_llama2"] = OllamaProvider(model="llama2")
        except Exception as e:
            logger.debug(f"Ollama not available: {e}")

        return providers


# ============================================================================
# EXAMPLE USAGE
# ============================================================================

if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO)

    # Example 1: Use OpenAI
    print("\n=== OpenAI Example ===")
    try:
        openai_provider = LLMFactory.create("openai")
        result = openai_provider.generate_patch(
            "Sort a list of numbers in Python"
        )
        print(f"Code:\n{result['code']}")
        print(f"Cost: ${result['cost']:.4f}")
    except Exception as e:
        print(f"Error: {e}")

    # Example 2: Use Claude
    print("\n=== Claude Example ===")
    try:
        claude_provider = LLMFactory.create("claude")
        result = claude_provider.generate_patch(
            "Implement binary search in Python"
        )
        print(f"Code:\n{result['code']}")
        print(f"Cost: ${result['cost']:.4f}")
    except Exception as e:
        print(f"Error: {e}")

    # Example 3: Create all providers
    print("\n=== All Providers ===")
    providers = LLMFactory.create_all()
    for name, provider in providers.items():
        print(f"✓ {name}: ${provider.get_cost_per_1k_tokens():.6f}/1K tokens")
