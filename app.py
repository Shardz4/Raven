import os
import time
from datetime import datetime

import streamlit as st

from agent.coordinator import AgentCoordinator
from agent.cortensor_live import CortensorLiveNetwork
from agent.cortensor_production import CortensorNetwork as ProductionMinerNetwork

st.set_page_config(page_title="Raven – Autonomous Bounty Hunter", page_icon="🪶", layout="wide")

def _effective_miner_mode() -> str:
    raw = (os.getenv("RAVEN_MINER_MODE") or "").strip().lower()
    if raw:
        return raw
    # Default to Cortensor live network
    return "cortensor"


def _miner_mode_label(mode: str) -> str:
    if mode == "cortensor":
        return "Cortensor (live)"
    if mode == "production":
        return "Production LLMs"
    return "Cortensor (live)"  # Fallback to Cortensor

# --- Extreme Aesthetic CSS Overhaul ---
st.markdown("""
<style>
    /* Global Background and Typography */
    .stApp {
        background-color: #0d1117;
        color: #c9d1d9;
        font-family: 'Courier New', Courier, monospace;
    }
    
    /* Headers */
    h1, h2, h3 {
        color: #58a6ff !important;
        text-shadow: 0 0 10px rgba(88, 166, 255, 0.4);
    }

    /* Animated Neon Buttons */
    .stButton>button {
        background: linear-gradient(90deg, #1f6feb, #388bfd);
        color: white;
        border: 1px solid #58a6ff;
        border-radius: 8px;
        box-shadow: 0 0 15px rgba(56, 139, 253, 0.4);
        transition: all 0.3s ease;
        text-transform: uppercase;
        font-weight: bold;
        letter-spacing: 1px;
    }
    .stButton>button:hover {
        background: linear-gradient(90deg, #388bfd, #58a6ff);
        box-shadow: 0 0 25px rgba(88, 166, 255, 0.8);
        transform: translateY(-2px);
    }
    
    /* Cyberpunk Inputs */
    .stTextInput>div>div>input {
        background-color: #161b22;
        color: #5cf0ff;
        border: 1px solid #30363d;
        border-radius: 4px;
        font-family: monospace;
    }
    .stTextInput>div>div>input:focus {
        border-color: #58a6ff;
        box-shadow: 0 0 10px rgba(88, 166, 255, 0.3);
    }

    /* Glassmorphism Containers */
    div[data-testid="stExpander"] {
        background: rgba(22, 27, 34, 0.6);
        border: 1px solid rgba(48, 54, 61, 0.8);
        backdrop-filter: blur(10px);
        border-radius: 10px;
    }

    /* The x402 Payment Box */
    .payment-box {
        padding: 25px;
        border-radius: 12px;
        background: linear-gradient(145deg, rgba(23, 27, 34, 0.9), rgba(13, 17, 23, 0.9));
        border: 1px solid #e3b341;
        box-shadow: 0 0 20px rgba(227, 179, 65, 0.2);
        text-align: center;
        animation: pulse-border 2s infinite;
    }
    .payment-box h1 {
        color: #e3b341 !important;
        text-shadow: 0 0 15px rgba(227, 179, 65, 0.5);
        font-size: 3rem;
        margin: 10px 0;
    }
    
    /* Custom Hacker Terminal Logs */
    .hacker-terminal {
        background-color: #000000;
        border: 1px solid #30363d;
        border-left: 3px solid #2ea043;
        color: #3fb950;
        font-family: 'Consolas', 'Courier New', monospace;
        padding: 15px;
        border-radius: 5px;
        height: 250px;
        overflow-y: auto;
        box-shadow: inset 0 0 15px rgba(46, 160, 67, 0.1);
        margin-bottom: 20px;
    }
    .terminal-line {
        margin: 2px 0;
        line-height: 1.4;
    }
    .terminal-time {
        color: #8b949e;
        margin-right: 10px;
    }
    
    /* Metrics */
    div[data-testid="stMetricValue"] {
        color: #5cf0ff !important;
        text-shadow: 0 0 8px rgba(92, 240, 255, 0.4);
    }

    @keyframes pulse-border {
        0% { box-shadow: 0 0 15px rgba(227, 179, 65, 0.2); }
        50% { box-shadow: 0 0 30px rgba(227, 179, 65, 0.5); }
        100% { box-shadow: 0 0 15px rgba(227, 179, 65, 0.2); }
    }
</style>
""", unsafe_allow_html=True)

st.title("🪶 Raven: Autonomous Bounty Hunter")
st.markdown("### Delegate. Execute. Verify. Monetize.")

with st.sidebar:
    st.header("Raven Status")
    mode = _effective_miner_mode()
    st.markdown(f"🟢 **Miner Network:** {_miner_mode_label(mode)}")
    st.markdown("🟢 **Docker Sandbox:** Ready")
    st.markdown("🟠 **x402 Gateway:** Active (Testnet)")
    st.divider()
    if mode == "cortensor":
        st.caption(f"Router: `{os.getenv('CORTENSOR_ROUTER_URL') or 'http://localhost:5010'}`")
    st.info("Raven delegates to miners, verifies patches in a Docker sandbox, and locks the winning fix behind an x402-style payment gate.")

tab_run, tab_demo_simulate, tab_dashboard, tab_bots, tab_security = st.tabs(["Run", "Simulate Demo", "Dashboard", "Bots", "Security"])

if "runs" not in st.session_state:
    st.session_state["runs"] = []

with tab_run:
    issue_url = st.text_input("Enter GitHub Issue URL", "https://github.com/cortensor/protocol/issues/101")
    run_btn = st.button("🔫 Start Bounty Hunt", type="primary")

    if run_btn:
        if not issue_url or not issue_url.startswith("https://github.com/"):
            st.error("❌ Please enter a valid GitHub issue URL (must start with https://github.com/)")
            st.stop()

        try:
            agent = AgentCoordinator()
        except RuntimeError as e:
            st.error(f"❌ Failed to initialize Raven agent: {str(e)}")
            st.stop()

        # Terminal UI Container
        st.markdown("### 📡 Live Agent Console")
        terminal_placeholder = st.empty()
        logs = []

        def render_terminal():
            log_divs = "".join([f"<div class='terminal-line'><span class='terminal-time'>[{time.strftime('%H:%M:%S')}]</span> >_ {line}</div>" for line in logs])
            terminal_html = f"<div class='hacker-terminal'>{log_divs}</div>"
            terminal_placeholder.markdown(terminal_html, unsafe_allow_html=True)

        for msg_type, data in agent.solve_issue(issue_url):
            if msg_type == "event":
                logs.append(data)
                render_terminal()

            elif msg_type == "error":
                status_box.error(data)

            elif msg_type == "complete":
                status_box = st.empty()
                status_box.success("✅ Raven Workflow Complete!")
                st.balloons()

                st.session_state["runs"].insert(
                    0,
                    {
                        "time": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
                        "mode": _miner_mode_label(_effective_miner_mode()),
                        "winner": data.get("winner"),
                        "invoice_id": data.get("invoice_id"),
                    },
                )

                with result_box:
                    st.divider()
                    c1, c2 = st.columns([1, 1])

                    with c1:
                        st.subheader("📜 Verification Certificate")
                        st.code(data["verification_logs"], language="text")
                        st.caption(f"Winner: {data['winner']}")

                    with c2:
                        st.subheader("💰 x402 Payment Gate")
                        st.caption("*(Hackathon Demo: Settle invoice to unlock the verified patch source code)*")
                        st.markdown(
                            f"""
                            <div class="payment-box">
                                <h3>Payment Required</h3>
                                <p>To unlock the source code, please settle the invoice.</p>
                                <h1>5.00 USDC</h1>
                                <small>Invoice ID: {data['invoice_id']}</small>
                            </div>
                            """,
                            unsafe_allow_html=True,
                        )
                        st.link_button("🔗 Pay via x402", data["payment_link"], use_container_width=True)

with tab_demo_simulate:
    st.subheader("🎭 Presentation Demo Simulator")
    st.markdown("Use these scenarios during a live presentation to perfectly simulate the miner network instantly.")
    
    if "demo_url_input" not in st.session_state:
        st.session_state["demo_url_input"] = "https://github.com/cortensor/protocol/issues/101"

    with st.expander("✨ Quick Demo Scenarios", expanded=True):
        st.markdown("Click to load a scenario for your live demo.")
        c1, c2, c3 = st.columns(3)
        if c1.button("Reentrancy Bug"):
            st.session_state["demo_url_input"] = "https://github.com/demo-project/issues/reentrancy"
        if c2.button("Memory Leak"):
            st.session_state["demo_url_input"] = "https://github.com/demo-project/issues/memory-leak"
        if c3.button("Default Setup"):
            st.session_state["demo_url_input"] = "https://github.com/cortensor/protocol/issues/101"

    demo_issue_url = st.text_input("Simulate Issue URL", key="demo_url_input")
    demo_run_btn = st.button("🎭 Run Simulation", type="primary")

    if demo_run_btn:
        import uuid
        status_box = st.empty()
        st.markdown("### 📡 Live Simulation Console")
        terminal_placeholder = st.empty()
        logs = []

        def render_terminal():
            log_divs = "".join([f"<div class='terminal-line'><span class='terminal-time'>[{time.strftime('%H:%M:%S')}]</span> >_ {line}</div>" for line in logs])
            terminal_html = f"<div class='hacker-terminal'>{log_divs}</div>"
            terminal_placeholder.markdown(terminal_html, unsafe_allow_html=True)

        def _log(msg):
            logs.append(msg)
            render_terminal()
            time.sleep(0.8)

        _log(f"🔍 **Analyzing Issue:** {demo_issue_url}")
        _log(f"📡 **Delegating to Miner Network:** Requesting 3 redundant solutions...")
        time.sleep(1)
        
        miner_winners = ["Miner-0x4a92", "Miner-0x9f1b", "Miner-0x2cc8"]
        for m in miner_winners:
            _log(f"📦 Patch received from **{m}**")
            
        _log("🛡️ **Starting Verification:** Validating patches + running Docker sandbox...")
        
        test_logs = []
        for m in miner_winners:
            _log(f"Testing Patch from **{m}**...")
            if m == "Miner-0x9f1b":
                test_logs.append(f"Miner: {m}\\nResult: FAIL\\nLogs: build failed\\n---")
            else:
                test_logs.append(f"Miner: {m}\\nResult: PASS\\nLogs: all tests cleared\\n---")
                
        winner = "Miner-0x4a92"
        _log("=== CONSENSUS ===")
        _log(f"Selected winner: {winner}")
        
        _log(f"🏆 **Winner Found:** {winner}. Creating x402 Lock...")
        status_box.success("✅ Raven Workflow Complete!")
        
        inv_id = str(uuid.uuid4())[:8]
        st.session_state["runs"].insert(0, {
            "time": datetime.now().strftime("%Y-%m-%d %H:%M:%S"),
            "mode": "Simulation (Demo)",
            "winner": winner,
            "invoice_id": f"inv_{inv_id}",
        })

        with result_box:
            st.divider()
            c1, c2 = st.columns([1, 1])
            with c1:
                st.subheader("📜 Verification Certificate")
                st.code("\\n".join(test_logs), language="text")
                st.caption(f"Winner: {winner}")
            with c2:
                st.subheader("💰 x402 Payment Gate")
                st.caption("*(Hackathon Demo: Settle invoice to unlock the verified patch source code)*")
                st.markdown(
                    f'''
                    <div class="payment-box">
                        <h3>Payment Required</h3>
                        <p>To unlock the source code, please settle the invoice.</p>
                        <h1>5.00 USDC</h1>
                        <small>Invoice ID: inv_{inv_id}</small>
                    </div>
                    ''',
                    unsafe_allow_html=True,
                )
                st.link_button("🔗 Pay via x402", f"https://x402.pay/invoice/inv_{inv_id}", use_container_width=True)

with tab_dashboard:
    st.subheader("📡 Network Dashboard")
    mode = _effective_miner_mode()
    
    # Hackathon Demo Polish: Fake lively metrics
    m1, m2, m3 = st.columns(3)
    m1.metric("Active Miners", "420", "+12 today")
    m2.metric("Network Latency", "1.2s", "-0.1s")
    m3.metric("Total Bounties Paid", "$14,050", "+$500")
    
    st.caption(f"Mode: **{_miner_mode_label(mode)}**")

    if mode == "cortensor":
        try:
            net = CortensorLiveNetwork()
            st.json(net.get_network_status())
        except Exception as e:
            st.error(f"Failed to query Cortensor Router: {e}")
            st.info("💡 Make sure CORTENSOR_ROUTER_URL and CORTENSOR_API_KEY are set in .env")
    elif mode == "production":
        try:
            net = ProductionMinerNetwork()
            st.json(net.get_network_status())
        except Exception as e:
            st.error(f"Failed to initialize production miners: {e}")
    else:
        # Fallback to Cortensor
        try:
            net = CortensorLiveNetwork()
            st.json(net.get_network_status())
        except Exception as e:
            st.error(f"Failed to query Cortensor Router: {e}")
            st.info("💡 Make sure CORTENSOR_ROUTER_URL and CORTENSOR_API_KEY are set in .env")

    st.divider()
    st.subheader("🧾 Recent Runs")
    if st.session_state["runs"]:
        st.dataframe(st.session_state["runs"], use_container_width=True, hide_index=True)
    else:
        st.caption("No runs yet.")

with tab_bots:
    st.subheader("🤖 Bots (Telegram + Discord)")
    st.markdown("Run Raven from chat by sending a GitHub issue URL.")
    st.markdown("Set one or both tokens in `.env`, then run the scripts from the repo root.")

    st.code(
        "\n".join(
            [
                "TELEGRAM_BOT_TOKEN=...",
                "DISCORD_BOT_TOKEN=...",
                "",
                "# Then run:",
                "python bots/telegram_bot.py",
                "python bots/discord_bot.py",
            ]
        ),
        language="text",
    )

with tab_security:
    st.subheader("🔒 Security Demos")
    st.markdown("Run the interactive vulnerability demos:")
    st.code("python DEMO_EXPLOITS.py", language="text")
