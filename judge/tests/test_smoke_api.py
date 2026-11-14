import importlib.util
import json
import os
from pathlib import Path
import signal
import sys
import tempfile
import threading
import time
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
spec = importlib.util.spec_from_file_location('api_smoke_watch', ROOT / 'smoke-api.py')
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


def sample(at, wait, verdict='AC', **extra):
    return dict(submittedAt=at, waitMs=wait, verdict=verdict, **extra)


class SummarizeTests(unittest.TestCase):
    def test_summarize_compares_against_the_baseline(self):
        before = [sample(1, w) for w in (900, 1100, 1000)]
        result = smoke.summarize(before + [sample(10, 1200), sample(11, 9000)], 10)
        self.assertEqual((result['status'], result['baselineWaitMs'], result['maxWaitMs']), ('passed', 1000, 9000))
        self.assertEqual(smoke.summarize(before + [sample(10, 12000)], 10)['status'], 'failed')

    def test_summarize_fails_on_rejection_or_je(self):
        samples = [sample(1, 1000), sample(10, 1000)]
        self.assertEqual(smoke.summarize(samples, 10)['status'], 'passed')
        self.assertEqual(smoke.summarize(samples + [dict(rejected=True, submittedAt=5)], 10)['status'], 'failed')
        self.assertEqual(smoke.summarize(samples, 10)['rejected'], 0)
        self.assertEqual(smoke.summarize(samples + [sample(12, 900, 'JE')], 10)['je'], 1)
        self.assertEqual(smoke.summarize(samples + [sample(12, 900, 'JE')], 10)['status'], 'failed')

    def test_summarize_needs_samples_on_both_sides(self):
        self.assertEqual(smoke.summarize([sample(10, 1)], 10)['status'], 'failed')
        self.assertEqual(smoke.summarize([sample(1, 1)], 10)['status'], 'failed')
        self.assertEqual(smoke.summarize([], 10)['status'], 'failed')

    def test_a_submission_still_waiting_at_the_switch_counts_after_it(self):
        # Submitted 2 s before the switch and started 15 s after submitting: the switch is what it waited through.
        before = [sample(1, w) for w in (900, 1100, 1000)]
        result = smoke.summarize(before + [sample(8, 15000), sample(10, 1000)], 10)
        self.assertEqual((result['status'], result['baselineWaitMs'], result['maxWaitMs']), ('failed', 1000, 15000))
        # One that started before the switch stays in the baseline.
        self.assertEqual(smoke.summarize(before + [sample(8, 1500), sample(10, 1000)], 10)['maxWaitMs'], 1000)

    def test_a_wait_of_exactly_ten_seconds_more_passes(self):
        self.assertEqual(smoke.summarize([sample(1, 1000), sample(10, 11000)], 10)['status'], 'passed')
        self.assertEqual(smoke.summarize([sample(1, 1000), sample(10, 11001)], 10)['status'], 'failed')


class WatchTests(unittest.TestCase):
    def run_watch(self, directory, stop_after=None, reject=False, stuck=False, extra=(), interval='0.05', switch_in=0.15, limited=False):
        """Run `main --watch` against fakes; returns (report, cleanup calls, exit)."""
        posts, deleted, polls, logins, sent = [], [], {}, [], {}
        # The API's limit (2 per 60 s per owner), scaled by the same factor as the test's shortened cadence.
        window = smoke.LIMIT_WINDOW * float(interval) / smoke.WATCH_CADENCE
        directory = Path(directory)
        (directory / 'switch-at').write_text(str(time.time() + switch_in))

        def request(base, method, path, data=None, token=''):
            if path == '/runtimes':
                return dict(maintenance=False, items=[dict(id='python314')])
            if path == '/auth/login':
                logins.append(1)
                return dict(access_token='secret-%d' % len(logins))
            if path == '/my/submissions':
                if reject:
                    raise RuntimeError('refused')
                if limited:
                    recent = [t for t in sent.get(token, []) if time.monotonic() - t < window]
                    sent[token] = recent + [time.monotonic()]
                    if len(recent) >= smoke.SUBMIT_LIMIT:
                        raise RuntimeError('429 submission_rate_limited')
                posts.append(data)
                return dict(id='s%d' % len(posts))
            if path.startswith('/my/submissions/'):
                polls[path] = polls.get(path, 0) + 1
                if stuck and path.endswith('s1'):
                    return dict(status='QUEUED')
                # The second poll sees RUNNING, the third DONE.
                return dict(status='QUEUED' if polls[path] < 2 else 'RUNNING' if polls[path] < 3 else 'DONE',
                            result=dict(verdict='AC'))
            if method == 'DELETE':
                deleted.append(path)
            return dict(version=1)

        argv = ['smoke-api.py', '--function', 'f', '--api-url', 'https://test.invalid', '--report', str(directory / 'r.json'),
                '--watch', '--stop-file', str(directory / 'stop'), '--switch-at-file', str(directory / 'switch-at'),
                '--interval', interval, *extra]
        timer = None
        if stop_after is not None:
            timer = threading.Timer(stop_after, lambda: (directory / 'stop').write_text(''))
            timer.start()
        try:
            with patch.object(sys, 'argv', argv), patch.object(smoke, 'request', side_effect=request), \
                    patch.object(smoke, 'aws', return_value={'Environment': {'Variables': {'COGNITO_USER_POOL_ID': 'pool'}}}), \
                    patch.object(smoke, 'cognito') as cognito, patch.object(smoke, 'POLL_INTERVAL', 0.01):
                smoke.main()
        finally:
            if timer:
                timer.cancel()
        report = json.loads((directory / 'r.json').read_text())
        return report, deleted, cognito

    def test_watch_records_waits_and_cleans_up(self):
        with tempfile.TemporaryDirectory() as name:
            report, deleted, cognito = self.run_watch(name, stop_after=0.5)
            self.assertEqual(report['status'], 'passed')
            self.assertGreaterEqual(len(report['submissions']), 5)
            self.assertTrue(all(s['started'] and s['verdict'] == 'AC' for s in report['submissions']))
            self.assertEqual((report['rejected'], report['je']), (0, 0))
            self.assertEqual(len(deleted), smoke.WATCH_USERS)
            self.assertTrue(report['stopped'])
            self.assertGreaterEqual(json.loads(Path(name, 'r.progress.json').read_text())['started'], 1)
            self.assertEqual(json.loads(Path(name, 'r.cleanup.json').read_text())['users'], [])
            self.assertIn('admin-delete-user', [c.args[1] for c in cognito.call_args_list])

    def test_watch_counts_refused_submissions(self):
        with tempfile.TemporaryDirectory() as name:
            report, _, _ = self.run_watch(name, stop_after=0.3, reject=True)
            self.assertEqual(report['status'], 'failed')
            self.assertGreater(report['rejected'], 0)

    def test_a_submission_still_waiting_counts_its_wait_so_far(self):
        with tempfile.TemporaryDirectory() as name:
            with patch.object(smoke, 'WATCH_GRACE', 0.1):
                report, _, _ = self.run_watch(name, stop_after=0.5, stuck=True)
            stuck = [s for s in report['submissions'] if s['id'] == 's1'][0]
            self.assertFalse(stuck['started'])
            self.assertGreaterEqual(stuck['waitMs'], 300)

    def test_rotation_keeps_every_user_under_the_submission_limit(self):
        with tempfile.TemporaryDirectory() as name:
            report, _, _ = self.run_watch(name, stop_after=1.3, limited=True)
            self.assertGreater(len(report['submissions']), 3 * smoke.WATCH_USERS)
            self.assertEqual(report['rejected'], 0)
            self.assertEqual(report['status'], 'passed')

    def test_a_single_user_would_hit_the_limit(self):
        # The fake enforces the limit: with one user the same cadence is refused, so the rotation is what avoids it.
        with tempfile.TemporaryDirectory() as name, patch.object(smoke, 'WATCH_USERS', 1):
            report, _, _ = self.run_watch(name, stop_after=0.6, limited=True)
            self.assertGreater(report['rejected'], 0)

    def test_the_hard_cap_ends_the_watch_without_a_stop_file(self):
        with tempfile.TemporaryDirectory() as name, patch.object(smoke, 'WATCH_CAP', 0.4), patch.object(smoke, 'WATCH_GRACE', 0.2):
            started = time.monotonic()
            report, deleted, _ = self.run_watch(name)
            self.assertLess(time.monotonic() - started, 5)
            self.assertTrue(report['capped'])
            self.assertFalse(report['stopped'])
            self.assertEqual(len(deleted), smoke.WATCH_USERS)
            self.assertEqual(report['cleanup'], 'complete')

    def test_sigterm_still_runs_cleanup(self):
        with tempfile.TemporaryDirectory() as name:
            timer = threading.Timer(0.3, os.kill, (os.getpid(), signal.SIGTERM))
            timer.start()
            before = signal.getsignal(signal.SIGTERM)
            started = time.monotonic()
            report, deleted, _ = self.run_watch(name)
            timer.cancel()
            self.assertLess(time.monotonic() - started, 5)
            self.assertTrue(report['terminated'])
            self.assertFalse(report['stopped'])
            self.assertEqual(len(deleted), smoke.WATCH_USERS)
            self.assertEqual(signal.getsignal(signal.SIGTERM), before)


if __name__ == '__main__':
    unittest.main()
