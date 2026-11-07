#!/usr/bin/env python3
"""Read-only audit tool for the Firefly III AI categorizer.

Every request this tool makes is a GET, so it is always safe to run against
production. It exists to answer three questions about a categorizer's output:

  summary       per-month unknown-payee rate, AI tag counts, category spread
  descriptions  the raw withdrawal descriptions, one per line, for reading
  pairs         description -> (destination_name, category) samples

Usage:
    python3 tools/audit.py summary --months 6
    python3 tools/audit.py descriptions --months 3 | head -40

Credentials are read from the environment, falling back to .env files next to
the project root. Only FIREFLY_URL and FIREFLY_PERSONAL_TOKEN are required.
"""

from __future__ import annotations

import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from collections import Counter, defaultdict
from datetime import date, timedelta

# Payee names that mean "the bank/importer had no idea who this was". Kept in
# sync with classifier.blankNames on the Go side.
PLACEHOLDER_PAYEES = {
    "",
    "-",
    "--",
    "no name",
    "(no name)",
    "unknown",
    "n/a",
    "n.a.",
    "unknown payee",
    "(unknown payee)",
    "(no payee)",
    "(unknown destination)",
    "unknown destination",
    "pos purchase",
    "pos purchase (card)",
    "card purchase",
    "purchase",
    "transfer",
    "(transfer)",
}


def load_env(paths: list[str]) -> None:
    """Populate os.environ from .env files without overriding real env vars.

    Follows the docker-compose quoting rules closely enough for our files: an
    unquoted value ends at the first ``#`` that starts a comment (i.e. preceded
    by whitespace), and quoted values are taken verbatim up to the closing
    quote. Without this, ``TAG_PREFIX=ai  # Prefix for tags...`` yields the
    whole tail as the prefix and every tag comparison silently fails.
    """
    for path in paths:
        if not os.path.isfile(path):
            continue
        with open(path, encoding="utf-8") as fh:
            for raw in fh:
                line = raw.strip()
                if not line or line.startswith("#") or "=" not in line:
                    continue
                key, value = line.split("=", 1)
                os.environ.setdefault(key.strip(), _env_value(value))


def _env_value(raw: str) -> str:
    value = raw.strip()
    if value[:1] in ('"', "'"):
        quote = value[0]
        end = value.find(quote, 1)
        return value[1:end] if end != -1 else value[1:]
    if " #" in value:
        value = value.split(" #", 1)[0]
    return value.strip()


def api_get(base: str, token: str, path: str, params: dict | None = None) -> dict:
    url = base.rstrip("/") + "/api/v1/" + path.lstrip("/")
    if params:
        url += "?" + urllib.parse.urlencode(params)
    request = urllib.request.Request(
        url,
        headers={"Authorization": f"Bearer {token}", "Accept": "application/json"},
    )
    with urllib.request.urlopen(request, timeout=60) as response:
        return json.load(response)


def fetch_withdrawals(base: str, token: str, start: str, limit: int = 100) -> list[dict]:
    """Fetch every withdrawal split since `start`, following pagination."""
    splits: list[dict] = []
    page = 1
    while True:
        payload = api_get(
            base,
            token,
            "transactions",
            {"type": "withdrawal", "start": start, "limit": limit, "page": page},
        )
        for group in payload.get("data") or []:
            attributes = group.get("attributes") or {}
            for split in attributes.get("transactions") or []:
                splits.append(split)
        pagination = (payload.get("meta") or {}).get("pagination") or {}
        if page >= int(pagination.get("total_pages") or 1):
            return splits
        page += 1


def is_placeholder(name: str | None) -> bool:
    return (name or "").strip().lower() in PLACEHOLDER_PAYEES


def cmd_summary(splits: list[dict], tag_prefix: str) -> int:
    per_month = defaultdict(lambda: {"total": 0, "unknown": 0})
    tag_counts = Counter()
    category_counts = Counter()
    uncategorized = 0

    for split in splits:
        month = (split.get("date") or "")[:7] or "unknown"
        bucket = per_month[month]
        bucket["total"] += 1
        if is_placeholder(split.get("destination_name")):
            bucket["unknown"] += 1

        for tag in split.get("tags") or []:
            if tag.startswith(tag_prefix + ":"):
                tag_counts[tag] += 1

        category = (split.get("category_name") or "").strip()
        if category:
            category_counts[category] += 1
        else:
            uncategorized += 1

    if not per_month:
        print("no withdrawals found in the requested window", file=sys.stderr)
        return 1

    print("## Unknown-payee rate per month")
    print(f"{'month':<10}{'withdrawals':>12}{'unknown payee':>15}{'rate':>8}")
    for month in sorted(per_month):
        bucket = per_month[month]
        rate = 100.0 * bucket["unknown"] / bucket["total"] if bucket["total"] else 0.0
        print(f"{month:<10}{bucket['total']:>12}{bucket['unknown']:>15}{rate:>7.1f}%")

    print()
    print("## AI tags")
    if tag_counts:
        for tag, count in tag_counts.most_common():
            print(f"  {tag:<24}{count:>6}")
    else:
        print("  (none)")

    print()
    print("## Categories")
    print(f"  {'(uncategorized)':<32}{uncategorized:>6}")
    for category, count in category_counts.most_common():
        print(f"  {category:<32}{count:>6}")

    return 0


def cmd_descriptions(splits: list[dict], unique: bool) -> int:
    seen: set[str] = set()
    for split in splits:
        description = (split.get("description") or "").strip()
        if not description:
            continue
        if unique:
            if description in seen:
                continue
            seen.add(description)
        print(description)
    return 0


def cmd_pairs(splits: list[dict], limit: int) -> int:
    seen: set[tuple[str, str, str]] = set()
    printed = 0
    for split in splits:
        row = (
            (split.get("description") or "").strip(),
            (split.get("destination_name") or "").strip(),
            (split.get("category_name") or "").strip(),
        )
        if row in seen:
            continue
        seen.add(row)
        description, destination, category = row
        print(f"{description}\t{destination or '-'}\t{category or '-'}")
        printed += 1
        if limit and printed >= limit:
            break
    return 0


def main(argv: list[str]) -> int:
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument("command", choices=("summary", "descriptions", "pairs"))
    parser.add_argument("--months", type=int, default=6, help="how far back to look (default 6)")
    parser.add_argument("--all", action="store_true", help="descriptions: keep duplicates")
    parser.add_argument("--limit", type=int, default=200, help="pairs: max rows (0 = all)")
    parser.add_argument("--env-file", action="append", default=[], help="extra .env file to read")
    args = parser.parse_args(argv)

    here = os.path.dirname(os.path.abspath(__file__))
    root = os.path.dirname(here)
    load_env([
        *args.env_file,
        os.path.join(root, ".env"),
        os.path.join(os.path.dirname(root), ".categorizer.env"),
    ])

    base = os.environ.get("FIREFLY_URL", "").strip()
    token = os.environ.get("FIREFLY_PERSONAL_TOKEN", "").strip()
    if not base or not token:
        print("audit: FIREFLY_URL and FIREFLY_PERSONAL_TOKEN must be set", file=sys.stderr)
        return 2

    tag_prefix = os.environ.get("TAG_PREFIX", "ai").strip() or "ai"
    start = (date.today() - timedelta(days=30 * args.months)).isoformat()

    try:
        splits = fetch_withdrawals(base, token, start)
    except urllib.error.HTTPError as exc:
        print(f"audit: Firefly III returned {exc.code} {exc.reason}", file=sys.stderr)
        return 2
    except urllib.error.URLError as exc:
        print(f"audit: cannot reach {base}: {exc.reason}", file=sys.stderr)
        return 2

    if args.command == "summary":
        return cmd_summary(splits, tag_prefix)
    if args.command == "descriptions":
        return cmd_descriptions(splits, unique=not args.all)
    return cmd_pairs(splits, args.limit)


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
