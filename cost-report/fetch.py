"""Reads monthly costs from Cost Explorer through the AWS CLI."""

import json
import subprocess

# Each call costs 0.01 USD, and one page normally holds every month.
MAX_PAGES = 5


def fetch(start, end, run=subprocess.run):
    """Returns ResultsByTime from every page. The response is never printed: it holds per-service amounts."""
    command = [
        "aws", "ce", "get-cost-and-usage",
        "--region", "us-east-1",
        "--time-period", f"Start={start},End={end}",
        "--granularity", "MONTHLY",
        "--metrics", "UnblendedCost",
        "--group-by", "Type=DIMENSION,Key=SERVICE", "Type=DIMENSION,Key=RECORD_TYPE",
        "--output", "json",
    ]
    results = []
    token = None
    for _ in range(MAX_PAGES):
        done = run(command + (["--next-page-token", token] if token else []), capture_output=True, text=True)
        if done.returncode != 0:
            raise RuntimeError(f"aws ce get-cost-and-usage failed ({done.returncode}): {done.stderr.strip()}")
        body = json.loads(done.stdout)
        results += body["ResultsByTime"]
        token = body.get("NextPageToken")
        if not token:
            return results
    raise RuntimeError(f"Cost Explorer returned more than {MAX_PAGES} pages")
