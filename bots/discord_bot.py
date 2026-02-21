import os
import re
import asyncio
from dotenv import load_dotenv

import discord

from agent.coordinator import AgentCoordinator

load_dotenv()

GITHUB_ISSUE_RE = re.compile(r"https://github\.com/\S+/issues/\d+")


def _extract_issue_url(text: str) -> str | None:
    if not text:
        return None
    m = GITHUB_ISSUE_RE.search(text)
    return m.group(0) if m else None


intents = discord.Intents.default()
intents.message_content = True
client = discord.Client(intents=intents)


@client.event
async def on_ready():
    print(f"Raven Discord bot logged in as {client.user}")


@client.event
async def on_message(message: discord.Message):
    if message.author == client.user:
        return

    issue_url = _extract_issue_url(message.content or "")
    if not issue_url:
        return

    await message.channel.send(f"🪶 Raven starting on: {issue_url}")

    loop = asyncio.get_running_loop()

    def run_workflow():
        agent = AgentCoordinator()
        for msg_type, data in agent.solve_issue(issue_url):
            if msg_type == "event":
                yield ("event", data)
            elif msg_type == "error":
                yield ("error", data)
                return
            elif msg_type == "complete":
                yield ("complete", data)
                return

    results = await loop.run_in_executor(None, lambda: list(run_workflow()))
    for msg_type, payload in results:
        if msg_type == "event":
            # avoid flooding: keep only some events
            continue
        if msg_type == "error":
            await message.channel.send(str(payload))
        if msg_type == "complete":
            winner = payload.get("winner", "Unknown")
            invoice_id = payload.get("invoice_id", "")
            payment_link = payload.get("payment_link", "")
            await message.channel.send(f"✅ Done. Winner: {winner}\nInvoice: {invoice_id}\nPay: {payment_link}")


def main() -> None:
    token = os.getenv("DISCORD_BOT_TOKEN")
    if not token:
        raise RuntimeError("DISCORD_BOT_TOKEN is not set")
    client.run(token)


if __name__ == "__main__":
    main()

