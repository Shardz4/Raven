# Raven Web3 Demo (Cortensor SDK-style)

This folder is a **minimal Web3 demo** showing how you would interact with Cortensor smart contracts using `ethers.js`, based on the Cortensor Web3 SDK reference (WIP).

It is **optional** and not required to run the main Raven Streamlit app.

## Setup

```bash
cd web3_demo
npm install
```

## Configure

Create a `.env` in `web3_demo/` (or export env vars):

- `PUBLIC_RPC_URL` – RPC URL
- `SESSION_V2_ADDRESS` – SessionV2 contract address
- `SESSION_QUEUE_V2_ADDRESS` – SessionQueueV2 contract address
- `PRIVATE_KEY` – signer key (use a test wallet)

## Run

```bash
node index.js
```

## What it does

- Prints sessions owned by your address (`getSessionsByAddress`)
- Prints miners assigned to a session (`getEphemeralNodes`)
- (Optional) shows how you’d call `create()` and `submit()` (commented)

If you have official contract addresses/ABIs from Cortensor, replace the ABI fragments in `abis/`.

