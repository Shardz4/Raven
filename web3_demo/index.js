import { ethers } from "ethers";
import fs from "node:fs";
import path from "node:path";

function mustEnv(name) {
  const v = process.env[name];
  if (!v) throw new Error(`${name} is required`);
  return v;
}

function loadAbi(relPath) {
  const p = path.join(process.cwd(), relPath);
  return JSON.parse(fs.readFileSync(p, "utf8"));
}

async function main() {
  const PUBLIC_RPC_URL = mustEnv("PUBLIC_RPC_URL");
  const SESSION_V2_ADDRESS = mustEnv("SESSION_V2_ADDRESS");
  const SESSION_QUEUE_V2_ADDRESS = mustEnv("SESSION_QUEUE_V2_ADDRESS");
  const PRIVATE_KEY = mustEnv("PRIVATE_KEY");

  const provider = new ethers.JsonRpcProvider(PUBLIC_RPC_URL);
  const wallet = new ethers.Wallet(PRIVATE_KEY, provider);

  const SessionV2ABI = loadAbi("abis/SessionV2.json");
  const SessionQueueV2ABI = loadAbi("abis/SessionQueueV2.json");

  const session = new ethers.Contract(SESSION_V2_ADDRESS, SessionV2ABI, wallet);
  const queue = new ethers.Contract(SESSION_QUEUE_V2_ADDRESS, SessionQueueV2ABI, wallet);

  const addr = await wallet.getAddress();
  console.log("Address:", addr);

  const sessions = await session.getSessionsByAddress(addr);
  console.log("Sessions:", sessions);

  if (sessions.length > 0) {
    const sessionId = Number(sessions[0].sessionId ?? sessions[0].id ?? 0);
    const miners = await session.getEphemeralNodes(sessionId);
    console.log(`Miners for session ${sessionId}:`, miners);

    const tasks = await queue.getTasksBySessionId(sessionId);
    console.log(`Tasks for session ${sessionId}:`, tasks.length);
  }

  // --- Optional: create a session (see Cortensor Web3 SDK reference) ---
  // const tx = await session.create(
  //   "Raven Session",
  //   "Raven web3 demo",
  //   addr,
  //   1, 3, 1, 0, 0,
  //   false,
  //   0,
  //   0,
  //   300,
  //   5
  // );
  // await tx.wait();
  // console.log("Session created:", tx.hash);

  // --- Optional: submit a task ---
  // await session.submit(
  //   0,
  //   0,
  //   JSON.stringify({ type: "chat", message: "Hello from Raven" }),
  //   0,
  //   "",
  //   [1024, 1, 1, 1, 0, 0],
  //   "raven-web3-demo"
  // );
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});

