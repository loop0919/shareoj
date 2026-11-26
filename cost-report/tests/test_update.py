import contextlib
import datetime
import io
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import update

RDS = "Amazon Relational Database Service"

CONFIG = {
    "start": "2026-09",
    "sponsors_url": None,
    "standing_note": None,
    "month_notes": {},
    "groups": [
        {"id": "database", "label": "データベース", "aws": "Amazon RDS", "services": [RDS]},
        {"id": "other", "label": "その他"},
        {"id": "tax", "label": "消費税", "record_types": ["Tax"]},
    ],
}
EMPTY = {"version": 1, "updated": None, "months": {}}


def result(start, end, amount, estimated=False):
    return {
        "TimePeriod": {"Start": start, "End": end},
        "Total": {},
        "Estimated": estimated,
        "Groups": [
            {"Keys": [RDS, "Usage"], "Metrics": {"UnblendedCost": {"Amount": amount, "Unit": "USD"}}},
            {"Keys": ["Tax", "Tax"], "Metrics": {"UnblendedCost": {"Amount": "6.41", "Unit": "USD"}}},
        ],
    }


SEPTEMBER = result("2026-09-01", "2026-10-01", "57.6912")
OCTOBER = result("2026-10-01", "2026-11-01", "83.2177")


class FakeFetch:
    def __init__(self, results=None, error=None):
        self.results = results or []
        self.error = error
        self.calls = []

    def __call__(self, start, end):
        self.calls.append((start, end))
        if self.error:
            raise self.error
        return self.results


class UpdateTest(unittest.TestCase):
    def setUp(self):
        self.dir = Path(self.enterContext(tempfile.TemporaryDirectory()))
        self.config = self.dir / "config.json"
        self.data = self.dir / "data" / "costs.json"
        self.site = self.dir / "site"
        self.data.parent.mkdir()
        self.write_config(CONFIG)
        self.data.write_text(json.dumps(EMPTY), encoding="utf-8")
        self.enterContext(mock.patch.dict(os.environ))
        os.environ.pop("GITHUB_OUTPUT", None)

    def write_config(self, config):
        self.config.write_text(json.dumps(config, ensure_ascii=False), encoding="utf-8")

    def run_update(self, fetcher, today="2026-11-08", *extra):
        out, err = io.StringIO(), io.StringIO()
        argv = ["--config", str(self.config), "--data", str(self.data), "--site", str(self.site), "--today", today, *extra]
        with contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            code = update.main(argv, fetcher=fetcher)
        return code, out.getvalue(), err.getvalue()

    def stored(self):
        return json.loads(self.data.read_text(encoding="utf-8"))

    def test_updates_data_and_site(self):
        fetcher = FakeFetch([SEPTEMBER, OCTOBER])
        code, out, _ = self.run_update(fetcher)
        self.assertEqual(code, 0)
        self.assertEqual(fetcher.calls, [("2026-09-01", "2026-11-01")])
        self.assertEqual(self.stored(), {"version": 1, "updated": "2026-11-08", "months": {
            "2026-09": {"total": "64.10", "rows": [{"group": "database", "amount": "57.69"}, {"group": "tax", "amount": "6.41"}]},
            "2026-10": {"total": "89.63", "rows": [{"group": "database", "amount": "83.22"}, {"group": "tax", "amount": "6.41"}]},
        }})
        html = (self.site / "index.html").read_text(encoding="utf-8")
        self.assertIn('<p class="total">$89.63</p>', html)
        self.assertIn("$64.10", html)
        self.assertIn("status=updated", out.splitlines())

    def test_unchanged_writes_site_but_not_data(self):
        self.run_update(FakeFetch([SEPTEMBER, OCTOBER]))
        before = self.data.read_bytes()
        (self.site / "index.html").unlink()
        code, out, _ = self.run_update(FakeFetch([SEPTEMBER, OCTOBER]), "2026-11-09")
        self.assertEqual(code, 0)
        self.assertIn("status=unchanged", out.splitlines())
        self.assertEqual(self.data.read_bytes(), before)
        self.assertIn("最終更新日: 2026年11月8日", (self.site / "index.html").read_text(encoding="utf-8"))

    def test_pending_writes_nothing(self):
        before = self.data.read_bytes()
        estimated = result("2026-10-01", "2026-11-01", "83.2177", estimated=True)
        code, out, _ = self.run_update(FakeFetch([SEPTEMBER, estimated]))
        self.assertEqual(code, 0)
        self.assertIn("status=pending", out.splitlines())
        self.assertEqual(self.data.read_bytes(), before)
        self.assertFalse(self.site.exists())

    def test_no_fetch_rebuilds_from_stored(self):
        self.run_update(FakeFetch([SEPTEMBER, OCTOBER]))
        (self.site / "index.html").unlink()
        fetcher = FakeFetch([SEPTEMBER])
        code, out, _ = self.run_update(fetcher, "2026-11-20", "--no-fetch")
        self.assertEqual(code, 0)
        self.assertEqual(fetcher.calls, [])
        self.assertIn("status=unchanged", out.splitlines())
        self.assertIn('<p class="total">$89.63</p>', (self.site / "index.html").read_text(encoding="utf-8"))

    def test_nothing_to_query(self):
        fetcher = FakeFetch([SEPTEMBER])
        code, out, _ = self.run_update(fetcher, "2026-09-20")
        self.assertEqual(code, 0)
        self.assertEqual(fetcher.calls, [])
        self.assertIn("status=unchanged", out.splitlines())
        self.assertIn("まだ載せられる月がありません。", (self.site / "index.html").read_text(encoding="utf-8"))

    def test_missing_data_file_starts_empty(self):
        self.data.unlink()
        code, _, _ = self.run_update(FakeFetch([SEPTEMBER]), "2026-10-08")
        self.assertEqual(code, 0)
        self.assertEqual(list(self.stored()["months"]), ["2026-09"])

    def test_fetch_error_is_nonzero_and_writes_nothing(self):
        before = self.data.read_bytes()
        code, out, err = self.run_update(FakeFetch(error=RuntimeError("denied")))
        self.assertNotEqual(code, 0)
        self.assertIn("denied", err)
        self.assertNotIn("status=", out)
        self.assertEqual(self.data.read_bytes(), before)
        self.assertFalse(self.site.exists())

    def test_leak_stops_publication(self):
        before = self.data.read_bytes()
        self.write_config({**CONFIG, "standing_note": "account 123456789012"})
        code, out, _ = self.run_update(FakeFetch([SEPTEMBER, OCTOBER]))
        self.assertNotEqual(code, 0)
        self.assertNotIn("status=", out)
        self.assertEqual(self.data.read_bytes(), before)
        self.assertFalse(self.site.exists())

    def run_at(self, moment, fetcher):
        argv = ["--config", str(self.config), "--data", str(self.data), "--site", str(self.site)]
        clock = lambda: datetime.datetime.fromisoformat(moment)
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return update.main(argv, fetcher=fetcher, now=clock)

    def test_updated_date_is_the_date_in_japan(self):
        # 16:05 UTC on the 26th is 01:05 on the 27th for the page's readers.
        self.assertEqual(self.run_at("2026-11-26T16:05:00+00:00", FakeFetch([SEPTEMBER, OCTOBER])), 0)
        self.assertEqual(self.stored()["updated"], "2026-11-27")
        self.assertIn("最終更新日: 2026年11月27日", (self.site / "index.html").read_text(encoding="utf-8"))

    def test_months_still_follow_utc_like_the_aws_bill(self):
        # 01:00 on December 1 in Japan: AWS has not ended November yet, so it must not be asked for.
        fetcher = FakeFetch([SEPTEMBER, OCTOBER])
        self.assertEqual(self.run_at("2026-11-30T16:00:00+00:00", fetcher), 0)
        self.assertEqual(fetcher.calls, [("2026-09-01", "2026-11-01")])
        self.assertEqual(self.stored()["updated"], "2026-12-01")

    def test_github_output(self):
        output = self.dir / "github-output"
        output.write_text("earlier=1\n", encoding="utf-8")
        os.environ["GITHUB_OUTPUT"] = str(output)
        self.run_update(FakeFetch([SEPTEMBER, OCTOBER]))
        self.assertEqual(output.read_text(encoding="utf-8"), "earlier=1\nstatus=updated\n")

    def test_stdout_has_no_amounts(self):
        _, out, err = self.run_update(FakeFetch([SEPTEMBER, OCTOBER]))
        for amount in ("57.69", "83.2", "64.10", "89.63", "6.41"):
            self.assertNotIn(amount, out + err)


if __name__ == "__main__":
    unittest.main()
