#!/usr/bin/env python3
"""Rehearse a categorizer run against real data without an LLM provider.

`DRY_RUN=true` makes the categorizer decide exactly as it normally would but
log the decision instead of writing to Firefly III. To check what the model
would actually be asked, point OPENAI_BASE_URL at the stub below: it answers
every call with a fixed category and records the request body, so the prompts
can be read back afterwards.

    ids     print journal ids of placeholder-payee withdrawals -- the ones a
            payee-grouping bug shows up on
    show    print destination_name / category / tags for the given ids
    stub    serve an OpenAI-compatible /chat/completions endpoint

Typical loop:

    python3 tools/dryrun.py stub &
    docker compose run --rm -e PORT=3000 -e DRY_RUN=true \
        -e OPENAI_BASE_URL=http://127.0.0.1:18080/v1 server
    python3 tools/dryrun.py ids
    curl -X POST localhost:3000/batch/run -H 'Content-Type: application/json' \
        -d '{"filter":{"transaction_ids":["528"]},"force":true}'
    python3 tools/dryrun.py show 528

Only GET requests are made to Firefly III; the stub writes nothing.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
from datetime import date, timedelta
from http.server import BaseHTTPRequestHandler, HTTPServer

import audit

DEFAULT_PORT = 18080
DEFAULT_LOG = "/tmp/stub-prompts.log"


def cmd_ids(base: str, token: str, days: int, limit: int) -> int:
    start = (date.today() - timedelta(days=days)).isoformat()
    splits = audit.fetch_withdrawals(base, token, start)
    candidates = [s for s in splits if audit.is_placeholder(s.get("destination_name"))]
    if limit:
        candidates = candidates[:limit]
    if not candidates:
        print("no placeholder-payee withdrawals in the window", file=sys.stderr)
        return 1
    print(" ".join(s["transaction_journal_id"] for s in candidates))
    for split in candidates:
        print(
            f"{split['transaction_journal_id']} | {split.get('destination_name')} | "
            f"{(split.get('description') or '')[:70]}",
            file=sys.stderr,
        )
    return 0


def cmd_show(base: str, token: str, ids: list[str]) -> int:
    for journal_id in ids:
        payload = audit.api_get(base, token, f"transactions/{journal_id}")
        for split in payload["data"]["attributes"]["transactions"]:
            print(json.dumps({
                "id": split["transaction_journal_id"],
                "destination_name": split["destination_name"],
                "category_name": split["category_name"],
                "tags": split["tags"],
            }, ensure_ascii=False))
    return 0



def make_handler(category: str, log_path: str, merchant: str | None = None) -> type[BaseHTTPRequestHandler]:
    class Handler(BaseHTTPRequestHandler):
        def do_POST(self) -> None:  # noqa: N802 - BaseHTTPRequestHandler API
            length = int(self.headers.get("Content-Length") or 0)
            raw = self.rfile.read(length) if length else b"{}"
            try:
                body = json.loads(raw)
            except ValueError:
                body = {"raw": raw.decode("utf-8", "replace")}
            with open(log_path, "a", encoding="utf-8") as fh:
                fh.write(json.dumps(body.get("messages", body), ensure_ascii=False) + "\n")

            decision = json.dumps({
                "outcome": "CLASSIFIED",
                "category": {"name": category, "confidence": "CLASSIFIED"},
                "reason": "stub",
            } | ({"merchant": merchant} if merchant else {}))
            data = json.dumps({
                "id": "stub",
                "object": "chat.completion",
                "created": 0,
                "model": "stub",
                "choices": [{
                    "index": 0,
                    "message": {"role": "assistant", "content": decision},
                    "finish_reason": "stop",
                }],
                "usage": {"prompt_tokens": 0, "completion_tokens": 0, "total_tokens": 0},
            }).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, *args) -> None:
            pass

    return Handler


def cmd_stub(port: int, category: str, log_path: str, merchant: str | None) -> int:
    print(
        f"stub listening on http://127.0.0.1:{port}/v1, prompts -> {log_path}",
        file=sys.stderr,
    )
    HTTPServer(("127.0.0.1", port), make_handler(category, log_path, merchant)).serve_forever()
    return 0


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("command", choices=("ids", "show", "stub"))
    parser.add_argument("args", nargs="*", help="show: journal ids")
    parser.add_argument("--days", type=int, default=45, help="ids: how far back to look (default 45)")
    parser.add_argument("--limit", type=int, default=5, help="ids: max ids to print (0 = all)")
    parser.add_argument("--port", type=int, default=DEFAULT_PORT, help="stub: listen port")
    parser.add_argument("--category", default="Groceries", help="stub: category to answer with")
    parser.add_argument(
        "--merchant",
        default=None,
        help="stub: also answer with this merchant name (tests merchant learning)",
    )
    parser.add_argument("--log", default=DEFAULT_LOG, help="stub: where to append request bodies")
    parser.add_argument("--env-file", action="append", default=[], help="extra .env file to read")
    args = parser.parse_args(argv)

    if args.command == "stub":
        return cmd_stub(args.port, args.category, args.log, args.merchant)

    here = os.path.dirname(os.path.abspath(__file__))
    root = os.path.dirname(here)
    audit.load_env([*args.env_file, os.path.join(root, ".env")])

    base = os.environ.get("FIREFLY_URL", "").strip()
    token = os.environ.get("FIREFLY_PERSONAL_TOKEN", "").strip()
    if not base or not token:
        print("dryrun: FIREFLY_URL and FIREFLY_PERSONAL_TOKEN must be set", file=sys.stderr)
        return 2

    if args.command == "show":
        if not args.args:
            print("dryrun: show needs at least one journal id", file=sys.stderr)
            return 2
        return cmd_show(base, token, args.args)
    return cmd_ids(base, token, args.days, args.limit)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
