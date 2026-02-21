"""
Live Cortensor Network – real Web2 API integration.

Uses the Cortensor Router Node REST API (docs.cortensor.network):
- GET /api/v1/sessions, GET /api/v1/miners
- POST /api/v1/completions (session_id, prompt, stream=false)

Set RAVEN_MINER_MODE=cortensor and CORTENSOR_ROUTER_URL + CORTENSOR_API_KEY to use.
"""

import os
import re
import json
import logging
import time
from typing import List, Dict, Optional
from dotenv import load_dotenv

load_dotenv()
logger = logging.getLogger(__name__)


def _extract_code(text: str) -> str:
    """Extract Python code from markdown or plain text."""
    if not text or not text.strip():
        return ""
    match = re.search(r"```(?:python)?\n(.*?)\n```", text, re.DOTALL)
    if match:
        return match.group(1).strip()
    match = re.search(r"`([^`]+)`", text)
    if match:
        return match.group(1).strip()
    return text.strip()


class CortensorLiveNetwork:
    """
    Production Cortensor network via Router Node Web2 API.
    Requires a running Router Node (e.g. http://<host>:5010) and Bearer token.
    """

    def __init__(
        self,
        base_url: Optional[str] = None,
        api_key: Optional[str] = None,
        session_id: Optional[int] = None,
        timeout: int = 90,
    ):
        self.base_url = (base_url or os.getenv("CORTENSOR_ROUTER_URL") or "http://localhost:5010").rstrip("/")
        self.api_key = api_key or os.getenv("CORTENSOR_API_KEY") or "default-dev-token"
        self.session_id = session_id if session_id is not None else _int_env("CORTENSOR_SESSION_ID", 0)
        self.timeout = timeout
        self._session_id_used: Optional[int] = None

    def _headers(self) -> Dict[str, str]:
        return {
            "Authorization": f"Bearer {self.api_key}",
            "Content-Type": "application/json",
        }

    def _get(self, path: str) -> Dict:
        import requests
        r = requests.get(f"{self.base_url}{path}", headers=self._headers(), timeout=15)
        r.raise_for_status()
        return r.json() if r.content else {}

    def _post(self, path: str, json: Dict) -> Dict:
        import requests
        r = requests.post(f"{self.base_url}{path}", headers=self._headers(), json=json, timeout=self.timeout)
        r.raise_for_status()
        return r.json() if r.content else {}

    def _post_stream(self, path: str, json_body: Dict):
        """
        Best-effort SSE client for stream=true.
        Yields decoded SSE 'data:' payloads (already parsed into text chunks when possible).
        """
        import requests

        with requests.post(
            f"{self.base_url}{path}",
            headers=self._headers(),
            json=json_body,
            timeout=self.timeout,
            stream=True,
        ) as r:
            r.raise_for_status()
            for raw_line in r.iter_lines(decode_unicode=True):
                if not raw_line:
                    continue
                line = raw_line.strip()
                if not line.startswith("data:"):
                    continue
                data = line[len("data:") :].strip()
                if data == "[DONE]":
                    break

                # Try OpenAI-style streaming JSON: choices[0].delta.content
                try:
                    obj = json.loads(data)
                except Exception:
                    yield data
                    continue

                chunk = ""
                if isinstance(obj, dict) and "choices" in obj and isinstance(obj["choices"], list) and obj["choices"]:
                    c0 = obj["choices"][0] or {}
                    if isinstance(c0, dict) and "delta" in c0 and isinstance(c0["delta"], dict):
                        chunk = c0["delta"].get("content") or ""
                    elif isinstance(c0, dict) and "message" in c0 and isinstance(c0["message"], dict):
                        chunk = c0["message"].get("content") or ""
                    elif isinstance(c0, dict) and "text" in c0:
                        chunk = c0.get("text") or ""
                elif isinstance(obj, dict) and "content" in obj:
                    chunk = str(obj.get("content") or "")

                if chunk:
                    yield chunk

    def get_info(self) -> Dict:
        """GET /api/v1/info"""
        return self._get("/api/v1/info")

    def get_status(self) -> Dict:
        """GET /api/v1/status"""
        return self._get("/api/v1/status")

    def get_miners(self) -> List[Dict]:
        """GET /api/v1/miners – list connected miners."""
        data = self._get("/api/v1/miners")
        if isinstance(data, list):
            return data
        if isinstance(data, dict) and "miners" in data:
            return data["miners"]
        return data.get("data", []) if isinstance(data, dict) else []

    def get_sessions(self) -> List[Dict]:
        """GET /api/v1/sessions – list active sessions."""
        data = self._get("/api/v1/sessions")
        if isinstance(data, list):
            return data
        if isinstance(data, dict) and "sessions" in data:
            return data["sessions"]
        return data.get("data", []) if isinstance(data, dict) else []

    def _resolve_session_id(self) -> int:
        """Use first available session or configured CORTENSOR_SESSION_ID."""
        try:
            sessions = self.get_sessions()
        except Exception as e:
            logger.warning("Could not list sessions: %s. Using session_id=%s", e, self.session_id)
            return self.session_id
        if sessions:
            # Prefer numeric id from first session
            first = sessions[0]
            if isinstance(first, dict) and "id" in first:
                return int(first["id"]) if first["id"] is not None else self.session_id
            if isinstance(first, dict) and "session_id" in first:
                return int(first["session_id"])
        return self.session_id

    def completion(self, prompt: str, stream: bool = False) -> Dict:
        """
        POST /api/v1/completions with session_id and prompt.
        Returns dict with parsed 'content' and raw response.
        """
        sid = self._resolve_session_id()
        self._session_id_used = sid
        body = {"session_id": sid, "prompt": prompt, "stream": stream, "timeout": self.timeout}
        raw = self._post("/api/v1/completions", body)
        content = self._parse_completion_content(raw)
        return {"content": content, "raw": raw}

    def _parse_completion_content(self, raw) -> str:
        content = ""
        if isinstance(raw, dict):
            if "choices" in raw and isinstance(raw["choices"], list) and len(raw["choices"]) > 0:
                c = raw["choices"][0]
                if isinstance(c, dict) and "message" in c:
                    content = (c["message"] or {}).get("content") or ""
                elif isinstance(c, dict) and "text" in c:
                    content = c.get("text") or ""
            elif "result" in raw:
                content = raw["result"]
            elif "content" in raw:
                content = raw["content"]
            elif "text" in raw:
                content = raw["text"]
        if not content and isinstance(raw, str):
            content = raw
        return content

    def request_patches(
        self,
        issue_description: str,
        issue_code: Optional[str] = None,
        redundancy: int = 3,
    ) -> List[Dict]:
        """
        Request code patches from the live Cortensor network by sending
        the same prompt multiple times (redundancy) to the Router.
        """
        return list(self.request_patches_stream(issue_description, issue_code=issue_code, redundancy=redundancy))

    def request_patches_stream(
        self,
        issue_description: str,
        issue_code: Optional[str] = None,
        redundancy: int = 3,
    ):
        """
        Generator version of request_patches for better UI progress.
        Yields one candidate patch at a time.
        """
        context = issue_description
        if issue_code:
            context += f"\n\nCurrent Code:\n{issue_code}"

        prompt = f"""GitHub Issue:
{context}

You are an expert Python programmer. Provide a Python fix for this issue.
Return ONLY valid Python code in a markdown code block (e.g. ```python ... ```).
No explanation outside the block."""

        miners = []
        try:
            miners = self.get_miners()
        except Exception as e:
            logger.warning("Could not list miners: %s", e)

        # Build miner ids for labels (use real miner list or placeholders)
        miner_labels = []
        if miners:
            for i, m in enumerate(miners[: redundancy]):
                mid = "unknown"
                if isinstance(m, dict):
                    mid = m.get("id") or m.get("miner_id") or m.get("address") or f"Miner_{i}"
                else:
                    mid = str(m)
                miner_labels.append(mid)
        while len(miner_labels) < redundancy:
            miner_labels.append(f"Cortensor_Miner_{len(miner_labels)}")

        for i in range(redundancy):
            try:
                logger.info("Requesting patch from Cortensor (request %d/%d)...", i + 1, redundancy)
                start = time.time()
                out = self.completion(prompt, stream=False)
                content = (out.get("content") or "").strip()
                code = _extract_code(content) or content
                if not code:
                    code = "def fix_issue(data):\n    return data  # no code extracted"
                elapsed = time.time() - start
                yield {
                    "miner_id": miner_labels[i] if i < len(miner_labels) else f"Cortensor_Miner_{i}",
                    "code": code,
                    "explanation": content,
                    "signature": f"cortensor_live_{i}_{int(start)}",
                    "tokens": {"total": 0},
                    "cost": 0.0,
                    "duration": f"{elapsed:.2f}s",
                    "timestamp": time.time(),
                }
                logger.info("Cortensor patch %d received in %.2fs", i + 1, elapsed)
            except Exception as e:
                logger.error("Cortensor request %d failed: %s", i + 1, e)
                continue

    def request_patches_stream_events(
        self,
        issue_description: str,
        issue_code: Optional[str] = None,
        redundancy: int = 3,
    ):
        """
        Like request_patches_stream, but also yields progress events:
        - ("event", <text>)
        - ("candidate", <patch dict>)
        """
        context = issue_description
        if issue_code:
            context += f"\n\nCurrent Code:\n{issue_code}"

        prompt = f"""GitHub Issue:
{context}

You are an expert Python programmer. Provide a Python fix for this issue.
Return ONLY valid Python code in a markdown code block (e.g. ```python ... ```).
No explanation outside the block."""

        miners = []
        try:
            miners = self.get_miners()
        except Exception as e:
            yield ("event", f"⚠️ Could not list miners: {e}")

        miner_labels = []
        if miners:
            for i, m in enumerate(miners[: redundancy]):
                mid = "unknown"
                if isinstance(m, dict):
                    mid = m.get("id") or m.get("miner_id") or m.get("address") or f"Miner_{i}"
                else:
                    mid = str(m)
                miner_labels.append(mid)
        while len(miner_labels) < redundancy:
            miner_labels.append(f"Cortensor_Miner_{len(miner_labels)}")

        use_stream = (os.getenv("CORTENSOR_STREAM") or "").strip() in ("1", "true", "True")

        for i in range(redundancy):
            yield ("event", f"🛰️ Cortensor completion {i+1}/{redundancy} started…")
            start = time.time()
            content = ""
            try:
                sid = self._resolve_session_id()
                self._session_id_used = sid
                body = {"session_id": sid, "prompt": prompt, "stream": bool(use_stream), "timeout": self.timeout}

                if use_stream:
                    last_emit = 0.0
                    for chunk in self._post_stream("/api/v1/completions", body):
                        content += str(chunk)
                        now = time.time()
                        if now - last_emit >= 2.0 and len(content) > 200:
                            last_emit = now
                            yield ("event", f"…streaming ({len(content)} chars received)")
                else:
                    raw = self._post("/api/v1/completions", body)
                    content = (self._parse_completion_content(raw) or "").strip()

                code = _extract_code(content) or content
                if not code:
                    code = "def fix_issue(data):\n    return data  # no code extracted"
                elapsed = time.time() - start
                yield (
                    "candidate",
                    {
                        "miner_id": miner_labels[i] if i < len(miner_labels) else f"Cortensor_Miner_{i}",
                        "code": code,
                        "explanation": content,
                        "signature": f"cortensor_live_{i}_{int(start)}",
                        "tokens": {"total": 0},
                        "cost": 0.0,
                        "duration": f"{elapsed:.2f}s",
                        "timestamp": time.time(),
                    },
                )
            except Exception as e:
                yield ("event", f"❌ Cortensor request {i+1} failed: {e}")

    def get_network_status(self) -> Dict:
        """Summarize Router status and miners for UI."""
        try:
            info = self.get_info()
            status = self.get_status()
            miners = self.get_miners()
        except Exception as e:
            return {"status": "error", "error": str(e), "miners": 0}
        return {
            "status": "operational",
            "router_info": info,
            "router_status": status,
            "miners": len(miners),
            "session_id": self._session_id_used or self.session_id,
        }


def _int_env(name: str, default: int) -> int:
    try:
        v = os.getenv(name)
        return int(v) if v is not None else default
    except ValueError:
        return default
