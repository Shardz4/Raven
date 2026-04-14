import os
import time
import json
import requests
import sseclient
from datetime import datetime

import streamlit as st
from streamlit_lottie import st_lottie

# Backend API URL
BACKEND_URL = os.getenv("RAVEN_BACKEND_URL", "http://localhost:8080")

st.set_page_config(page_title="Raven – Autonomous AI Developer", page_icon="🪶", layout="wide")


@st.cache_data
def load_lottieurl(url: str):
    try:
        r = requests.get(url, timeout=5)
        if r.status_code != 200:
            return None
        return r.json()
    except Exception:
        return None


def api_get(path):
    """GET from the Go backend."""
    try:
        r = requests.get(f"{BACKEND_URL}{path}", timeout=10)
        r.raise_for_status()
        return r.json()
    except Exception as e:
        return {"error": str(e)}


def api_post(path, data):
    """POST to the Go backend."""
    try:
        r = requests.post(f"{BACKEND_URL}{path}", json=data, timeout=10)
        r.raise_for_status()
        return r.json()
    except Exception as e:
        return {"error": str(e)}


# ── CSS ──
st.markdown("""
<link href="https://fonts.googleapis.com/css2?family=Inter:wght@400;500;600;700&display=swap" rel="stylesheet">
<style>
    .stApp {
        background-color: #0d1117;
        color: #e6edf3;
        font-family: 'Inter', -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
    }
    h1, h2, h3 {
        color: #58a6ff !important;
        font-family: 'Inter', sans-serif;
        font-weight: 700;
    }
    .stButton>button {
        background: linear-gradient(135deg, #238636 0%, #2ea043 100%);
        color: white;
        border: none;
        border-radius: 8px;
        transition: all 0.2s ease;
        font-weight: 600;
        padding: 0.6rem 1.2rem;
        box-shadow: 0 2px 8px rgba(35, 134, 54, 0.3);
    }
    .stButton>button:hover {
        background: linear-gradient(135deg, #2ea043 0%, #3fb950 100%);
        box-shadow: 0 4px 16px rgba(46, 160, 67, 0.4);
        transform: translateY(-1px);
    }
    .stTextInput>div>div>input {
        background-color: #161b22;
        color: #e6edf3;
        border: 1px solid #30363d;
        border-radius: 8px;
        font-family: 'Inter', sans-serif;
    }
    .stTextInput>div>div>input:focus {
        border-color: #58a6ff;
        box-shadow: 0 0 0 3px rgba(88, 166, 255, 0.15);
    }
    div[data-testid="stExpander"] {
        background: #161b22;
        border: 1px solid #30363d;
        border-radius: 8px;
    }
    div[data-testid="stMetricValue"] {
        color: #58a6ff !important;
        font-weight: 700;
    }
    .console-terminal {
        background-color: #010409;
        border: 1px solid #30363d;
        color: #e6edf3;
        font-family: 'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace;
        padding: 16px;
        border-radius: 8px;
        height: 350px;
        overflow-y: auto;
        margin-bottom: 20px;
        font-size: 13px;
        line-height: 1.6;
    }
    .terminal-line { margin: 3px 0; }
    .terminal-time { color: #8b949e; margin-right: 8px; font-size: 11px; }
    .score-badge {
        display: inline-block;
        background: linear-gradient(135deg, #1f6feb 0%, #388bfd 100%);
        color: white;
        padding: 4px 12px;
        border-radius: 12px;
        font-size: 14px;
        font-weight: 600;
        margin: 2px;
    }
    .winner-card {
        background: linear-gradient(145deg, rgba(35, 134, 54, 0.1), rgba(46, 160, 67, 0.05));
        border: 1px solid #238636;
        border-radius: 12px;
        padding: 20px;
        margin-top: 16px;
    }
</style>
""", unsafe_allow_html=True)

st.title("🪶 Raven: Autonomous AI Developer")
st.markdown("### Resolve GitHub Issues using AI Ensemble + RavenMind Consensus")

# ── Sidebar ──
with st.sidebar:
    st.header("System Status")

    # Check backend health
    health = api_get("/api/health")
    if "error" in health:
        st.markdown("🔴 **Backend:** Offline")
        st.error(f"Cannot reach backend at {BACKEND_URL}")
        st.info("Start the Go backend: `cd backend && go run .`")
    else:
        st.markdown(f"🟢 **Backend:** {health.get('status', 'unknown')}")

        # Show providers
        providers = api_get("/api/providers")
        if "solvers" in providers:
            st.markdown(f"🟢 **AI Models:** {len(providers['solvers'])}")
            for s in providers["solvers"]:
                st.caption(f"  - `{s['name']}/{s['model']}`")
            judge = providers.get("judge", {})
            st.markdown(f"⚖️ **Judge:** `{judge.get('name', '?')}/{judge.get('model', '?')}`")

    st.divider()
    st.info("Raven coordinates multiple AI models in parallel, tests their patches in Docker, "
            "and uses the RavenMind 4-phase consensus to select the best solution.")

# ── Tabs ──
tab_run, tab_dashboard = st.tabs(["🚀 Run Agent", "📊 Dashboard"])

if "runs" not in st.session_state:
    st.session_state["runs"] = []

with tab_run:
    c1, c2 = st.columns([3, 1])
    with c1:
        issue_url = st.text_input("Enter GitHub Issue URL", "https://github.com/microsoft/vscode/issues/101")
        run_btn = st.button("🚀 Start Resolution", type="primary")
    with c2:
        lottie_ai = load_lottieurl("https://assets9.lottiefiles.com/packages/lf20_tno6cg2w.json")
        if lottie_ai:
            st_lottie(lottie_ai, height=100, key="run_header_anim")

    if run_btn:
        if not issue_url or not issue_url.startswith("https://github.com/"):
            st.error("❌ Please enter a valid GitHub issue URL")
            st.stop()

        # Submit to backend
        resp = api_post("/api/solve", {"issue_url": issue_url})
        if "error" in resp:
            st.error(f"❌ {resp['error']}")
            st.stop()

        job_id = resp.get("job_id")
        if not job_id:
            st.error("❌ Backend returned no job ID")
            st.stop()

        status_box = st.empty()
        status_box.info(f"⏳ Job `{job_id}` submitted. Streaming events...")

        st.markdown("### 🧠 RavenMind Live Console")
        terminal_placeholder = st.empty()
        logs = []

        def render_terminal():
            log_divs = "".join([
                f"<div class='terminal-line'><span class='terminal-time'>[{time.strftime('%H:%M:%S')}]</span> {line}</div>"
                for line in logs
            ])
            terminal_html = f"<div class='console-terminal'>{log_divs}</div>"
            terminal_placeholder.markdown(terminal_html, unsafe_allow_html=True)

        # Stream SSE events from backend
        try:
            stream_url = f"{BACKEND_URL}/api/solve/{job_id}/stream"
            response = requests.get(stream_url, stream=True, timeout=300)
            client = sseclient.SSEClient(response)

            for event in client.events():
                data = event.data
                if data == "[DONE]":
                    break
                logs.append(data)
                render_terminal()
        except Exception as e:
            logs.append(f"⚠️ Stream ended: {e}")
            render_terminal()

        # Fetch final result
        result = api_get(f"/api/solve/{job_id}")
        if result.get("status") == "completed":
            status_box.success(f"✅ Resolution complete! Winner: **{result.get('winner_model', '?')}**")
            st.balloons()

            st.session_state["runs"].insert(0, {
                "time": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                "issue": issue_url,
                "winner": result.get("winner_model", "?"),
                "job_id": job_id,
            })

            st.divider()
            st.markdown("<div class='winner-card'>", unsafe_allow_html=True)
            st.subheader("🎉 Verified Patch")
            st.markdown(result.get("explanation", ""))
            st.code(result.get("winner_code", ""), language="python")
            st.markdown("</div>", unsafe_allow_html=True)

            with st.expander("📊 RavenMind Consensus Report"):
                st.code(result.get("verification_logs", ""), language="text")

            with st.expander("📋 Full Job Details (JSON)"):
                st.json(result)
        elif result.get("status") == "failed":
            status_box.error(f"❌ {result.get('error_message', 'Unknown failure')}")
        else:
            status_box.warning(f"⏳ Job still processing (status: {result.get('status', '?')})")

with tab_dashboard:
    st.subheader("📊 Execution Dashboard")

    jobs = api_get("/api/jobs")
    if isinstance(jobs, list):
        m1, m2, m3 = st.columns(3)
        completed = [j for j in jobs if j.get("status") == "completed"]
        m1.metric("Total Jobs", str(len(jobs)))
        m2.metric("Successful", str(len(completed)))
        m3.metric("Success Rate", f"{len(completed)/max(len(jobs),1)*100:.0f}%")

        st.divider()
        st.subheader("🧾 Job History")
        if jobs:
            display_data = [{
                "ID": j["id"],
                "Issue": j.get("issue_title", j.get("issue_url", "?")),
                "Status": j["status"],
                "Winner": j.get("winner_model", "-"),
                "Created": j.get("created_at", ""),
            } for j in jobs]
            st.dataframe(display_data, use_container_width=True, hide_index=True)
        else:
            st.caption("No jobs yet. Submit your first issue!")
    else:
        st.warning("Could not fetch job history from backend.")
        if "error" in jobs:
            st.caption(jobs["error"])
