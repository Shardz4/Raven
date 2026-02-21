import os
import re
import time
import asyncio
from dotenv import load_dotenv

from telegram import Update
from telegram.ext import (
    ApplicationBuilder,
    CommandHandler,
    ContextTypes,
    MessageHandler,
    filters,
)

from agent.coordinator import AgentCoordinator

load_dotenv()

GITHUB_ISSUE_RE = re.compile(r"https://github\.com/\S+/issues/\d+")


def _extract_issue_url(text: str) -> str | None:
    if not text:
        return None
    m = GITHUB_ISSUE_RE.search(text)
    return m.group(0) if m else None


async def start_cmd(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    await update.message.reply_text(
        "Send me a GitHub Issue URL and I’ll run Raven on it.\n"
        "Example: https://github.com/cortensor/protocol/issues/101"
    )


async def handle_message(update: Update, context: ContextTypes.DEFAULT_TYPE) -> None:
    if not update.message:
        return
    issue_url = _extract_issue_url(update.message.text or "")
    if not issue_url:
        await update.message.reply_text("Please send a valid GitHub issue URL.")
        return

    chat_id = update.message.chat_id
    await update.message.reply_text(f"🪶 Raven starting on: {issue_url}")

    loop = asyncio.get_running_loop()

    def run_workflow():
        agent = AgentCoordinator()
        last_sent = 0.0
        for msg_type, data in agent.solve_issue(issue_url):
            now = time.time()
            if msg_type == "event":
                # throttle spam
                if now - last_sent >= 1.5:
                    last_sent = now
                    yield ("event", data)
            elif msg_type == "error":
                yield ("error", data)
                return
            elif msg_type == "complete":
                yield ("complete", data)
                return

    for item in await loop.run_in_executor(None, lambda: list(run_workflow())):
        msg_type, payload = item
        if msg_type == "event":
            await context.bot.send_message(chat_id=chat_id, text=str(payload))
        elif msg_type == "error":
            await context.bot.send_message(chat_id=chat_id, text=str(payload))
        elif msg_type == "complete":
            winner = payload.get("winner", "Unknown")
            invoice_id = payload.get("invoice_id", "")
            payment_link = payload.get("payment_link", "")
            await context.bot.send_message(
                chat_id=chat_id,
                text=f"✅ Done. Winner: {winner}\nInvoice: {invoice_id}\nPay: {payment_link}",
            )


def main() -> None:
    token = os.getenv("TELEGRAM_BOT_TOKEN")
    if not token:
        raise RuntimeError("TELEGRAM_BOT_TOKEN is not set")

    app = ApplicationBuilder().token(token).build()
    app.add_handler(CommandHandler("start", start_cmd))
    app.add_handler(MessageHandler(filters.TEXT & ~filters.COMMAND, handle_message))
    app.run_polling()


if __name__ == "__main__":
    main()

