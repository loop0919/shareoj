#!/usr/bin/env python3
"""Updates the stored months from Cost Explorer and builds the running-cost page.

Prints status=updated, unchanged or pending. On pending or on an error nothing is written.
"""

import argparse
import datetime
import json
import os
from pathlib import Path
import sys

import costs
import fetch
import page

EMPTY = {"version": 1, "updated": None, "months": {}}


def _report(status):
    line = f"status={status}"
    print(line)
    output = os.environ.get("GITHUB_OUTPUT")
    if output:
        with open(output, "a", encoding="utf-8") as stream:
            stream.write(line + "\n")


def main(argv, fetcher=fetch.fetch):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--config", type=Path, required=True)
    parser.add_argument("--data", type=Path, required=True, help="costs.json on the data branch")
    parser.add_argument("--site", type=Path, required=True, help="directory that receives index.html")
    parser.add_argument("--today", type=datetime.date.fromisoformat, help="defaults to today in UTC")
    parser.add_argument("--no-fetch", action="store_true", help="rebuild the page from the stored months")
    args = parser.parse_args(argv)
    today = args.today or datetime.datetime.now(datetime.timezone.utc).date()

    try:
        config = json.loads(args.config.read_text(encoding="utf-8"))
        stored = json.loads(args.data.read_text(encoding="utf-8")) if args.data.exists() else EMPTY
        period = None if args.no_fetch else costs.query_period(config["start"], today)
        fresh = costs.summarize(fetcher(*period), config) if period else {}
        data, changed = costs.merge(stored, fresh, today)
        html = page.render(data, config)
        text = json.dumps(data, ensure_ascii=False, indent=2, sort_keys=True) + "\n"
        costs.assert_public(html)
        costs.assert_public(text)
    except costs.Pending as pending:
        print("AWS has not finalized: " + ", ".join(pending.months), file=sys.stderr)
        _report("pending")
        return 0
    except (RuntimeError, ValueError, KeyError, OSError) as error:
        print(f"cost report failed: {error}", file=sys.stderr)
        return 1

    args.site.mkdir(parents=True, exist_ok=True)
    (args.site / "index.html").write_text(html, encoding="utf-8")
    if changed:
        args.data.parent.mkdir(parents=True, exist_ok=True)
        args.data.write_text(text, encoding="utf-8")
    _report("updated" if changed else "unchanged")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
