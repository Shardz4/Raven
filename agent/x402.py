import uuid
import time
import os
from dotenv import load_dotenv
import requests

load_dotenv()


class X402Merchant:
    def __init__(self):
        # In production, this would connect to an L402/Lightning node
        self.active_invoices = {}
        # Load x402 gateway URL from environment, with fallback
        self.gateway_url = os.getenv(
            "X402_GATEWAY_URL",
            "https://x402.pay/invoice",
        )
        # Mode: "demo" keeps auto-approve behavior, "production" requires real verification
        self.mode = os.getenv("X402_MODE", "demo").lower()

    def create_locked_content(self, content, price_usdc=5.00):
        """
        Gates content behind a payment request.
        """
        # In demo mode we keep a short id for presentation and exploit demos.
        # In production mode, use a full UUID to avoid brute-forceable invoice IDs.
        invoice_id = str(uuid.uuid4()) if self.mode == "production" else str(uuid.uuid4())[:8]
        payment_link = f"{self.gateway_url}/{invoice_id}?amount={price_usdc}"

        self.active_invoices[invoice_id] = {
            "content": content,
            "status": "unpaid",
            "price": price_usdc,
            "created_at": time.time(),
        }

        return {
            "invoice_id": invoice_id,
            "payment_link": payment_link,
            "status": "402 Payment Required",
        }

    def verify_payment(self, invoice_id):
        """
        Checks if the invoice has been paid.

        - demo mode: auto-approves (hackathon demo)
        - production mode: queries the configured X402 gateway
        """
        invoice = self.active_invoices.get(invoice_id)
        if not invoice:
            return None

        if self.mode == "demo":
            self.active_invoices[invoice_id]["status"] = "paid"
            return True

        # Production: ask the external x402 gateway about payment status.
        # The exact API shape depends on your gateway; this assumes a simple JSON
        # endpoint that returns {"paid": true/false} or {"status": "paid"}.
        try:
            resp = requests.get(f"{self.gateway_url}/{invoice_id}/status", timeout=5)
            resp.raise_for_status()
            data = resp.json()
            paid = bool(
                data.get("paid")
                or str(data.get("status", "")).lower() == "paid"
            )
            if paid:
                self.active_invoices[invoice_id]["status"] = "paid"
            return paid
        except Exception:
            return False

    def retrieve_content(self, invoice_id):
        """
        Returns content ONLY if paid.
        """
        invoice = self.active_invoices.get(invoice_id)
        if invoice and invoice["status"] == "paid":
            return invoice["content"]
        raise PermissionError("402 Payment Required: Content is locked.")
