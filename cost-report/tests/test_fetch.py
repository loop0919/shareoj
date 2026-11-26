import json
from pathlib import Path
import subprocess
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import fetch


def page_of(start, token=None):
    body = {
        "ResultsByTime": [{"TimePeriod": {"Start": start, "End": "2026-12-01"}, "Total": {}, "Groups": [], "Estimated": False}],
        "DimensionValueAttributes": [],
    }
    if token:
        body["NextPageToken"] = token
    return body


class FakeAws:
    def __init__(self, replies):
        self.replies = list(replies)
        self.calls = []

    def __call__(self, argv, capture_output, text):
        self.calls.append(argv)
        reply = self.replies[min(len(self.calls), len(self.replies)) - 1]
        if isinstance(reply, subprocess.CompletedProcess):
            return reply
        return subprocess.CompletedProcess(argv, 0, stdout=json.dumps(reply), stderr="")


class FetchTest(unittest.TestCase):
    def test_command(self):
        aws = FakeAws([page_of("2026-09-01")])
        fetch.fetch("2026-09-01", "2026-12-01", run=aws)
        self.assertEqual(aws.calls, [[
            "aws", "ce", "get-cost-and-usage",
            "--region", "us-east-1",
            "--time-period", "Start=2026-09-01,End=2026-12-01",
            "--granularity", "MONTHLY",
            "--metrics", "UnblendedCost",
            "--group-by", "Type=DIMENSION,Key=SERVICE", "Type=DIMENSION,Key=RECORD_TYPE",
            "--output", "json",
        ]])

    def test_follows_next_page_token(self):
        aws = FakeAws([page_of("2026-09-01", token="t1"), page_of("2026-10-01")])
        results = fetch.fetch("2026-09-01", "2026-12-01", run=aws)
        self.assertEqual(len(aws.calls), 2)
        self.assertNotIn("--next-page-token", aws.calls[0])
        self.assertEqual(aws.calls[1][-2:], ["--next-page-token", "t1"])
        self.assertEqual([result["TimePeriod"]["Start"] for result in results], ["2026-09-01", "2026-10-01"])

    def test_stops_after_five_pages(self):
        aws = FakeAws([page_of("2026-09-01", token="again")])
        with self.assertRaises(RuntimeError):
            fetch.fetch("2026-09-01", "2026-12-01", run=aws)
        self.assertEqual(len(aws.calls), 5)

    def test_cli_failure_raises_without_stdout(self):
        failed = subprocess.CompletedProcess([], 255, stdout="SECRET-BODY", stderr="denied")
        with self.assertRaises(RuntimeError) as caught:
            fetch.fetch("2026-09-01", "2026-12-01", run=FakeAws([failed]))
        self.assertIn("denied", str(caught.exception))
        self.assertNotIn("SECRET-BODY", str(caught.exception))


if __name__ == "__main__":
    unittest.main()
