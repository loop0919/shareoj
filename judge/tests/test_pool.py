import http.server
import io
import json
import logging
import sys
import threading
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import pool
import telemetry


class Clock:
    def __init__(self):
        self.monotonic = 0.0
        self.wall = 1_800_000_000.0

    def advance(self, seconds):
        self.monotonic += seconds
        self.wall += seconds


def iso(seconds):
    import datetime
    return datetime.datetime.fromtimestamp(seconds, datetime.timezone.utc).isoformat().replace('+00:00', 'Z')


class IdleStopTests(unittest.TestCase):
    def setUp(self):
        self.output = io.StringIO()
        self.old_handlers = telemetry.logger.handlers[:]
        telemetry.logger.handlers = [logging.StreamHandler(self.output)]
        telemetry.logger.setLevel(logging.INFO)
        self.clock = Clock()
        self.tags = dict.fromkeys(pool.HOLD_TAGS)
        self.reads = 0

    def tearDown(self):
        telemetry.logger.handlers = self.old_handlers

    def read(self):
        self.reads += 1
        if isinstance(self.tags, Exception):
            raise self.tags
        return dict(self.tags)

    def stopper(self):
        return pool.IdleStop(self.read, monotonic=lambda: self.clock.monotonic, wall=lambda: self.clock.wall)

    def test_role_selects_behaviour(self):
        for role in ('', 'primary'):
            with self.subTest(role=role), patch.dict('os.environ', {'JUDGE_POOL_ROLE': role}):
                self.assertIsNone(pool.IdleStop.from_environment())
        with patch.dict('os.environ', {'JUDGE_POOL_ROLE': 'burst'}):
            self.assertIsInstance(pool.IdleStop.from_environment(), pool.IdleStop)
        with patch.dict('os.environ', {'JUDGE_POOL_ROLE': 'spare'}), self.assertRaises(ValueError):
            pool.IdleStop.from_environment()

    def test_metadata_endpoint_follows_sdk_settings(self):
        with patch.dict('os.environ', {'AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE': 'IPv6'}, clear=True):
            self.assertEqual(pool.metadata_endpoint(), 'http://[fd00:ec2::254]')
        with patch.dict('os.environ', {}, clear=True):
            self.assertEqual(pool.metadata_endpoint(), 'http://169.254.169.254')
        with patch.dict('os.environ', {'AWS_EC2_METADATA_SERVICE_ENDPOINT': 'http://[::1]:8080/'}, clear=True):
            self.assertEqual(pool.metadata_endpoint(), 'http://[::1]:8080')

    def test_recent_work_keeps_the_host_without_reading_metadata(self):
        stopper = self.stopper()
        self.clock.advance(pool.IDLE_SECONDS - 1)
        self.assertFalse(stopper.due())
        self.clock.advance(1)
        stopper.touch()
        self.clock.advance(pool.IDLE_SECONDS - 1)
        self.assertFalse(stopper.due())
        self.assertEqual(self.reads, 0)

    def test_idle_host_without_holds_stops(self):
        stopper = self.stopper()
        self.clock.advance(pool.IDLE_SECONDS)
        self.assertTrue(stopper.due())

    def test_later_hold_wins_and_needs_grace(self):
        stopper = self.stopper()
        self.clock.advance(pool.IDLE_SECONDS)
        self.tags = dict(JudgeHoldUntil=iso(self.clock.wall - 3600), JudgeMaintenanceHoldUntil=iso(self.clock.wall + 120))
        self.assertFalse(stopper.due())
        self.clock.advance(120 + pool.GRACE_SECONDS - 1)
        self.clock.advance(pool.CHECK_SECONDS)
        self.tags['JudgeMaintenanceHoldUntil'] = iso(self.clock.wall - pool.GRACE_SECONDS + 1)
        self.assertFalse(stopper.due())
        self.clock.advance(pool.CHECK_SECONDS)
        self.assertTrue(stopper.due())

    def test_metadata_is_read_at_most_once_a_minute(self):
        stopper = self.stopper()
        self.tags = dict(JudgeHoldUntil=iso(self.clock.wall + 3600), JudgeMaintenanceHoldUntil=None)
        self.clock.advance(pool.IDLE_SECONDS)
        self.assertFalse(stopper.due())
        self.clock.advance(pool.CHECK_SECONDS - 1)
        self.assertFalse(stopper.due())
        self.assertEqual(self.reads, 1)

    def test_unreadable_or_invalid_hold_keeps_the_host_and_reports_once(self):
        stopper = self.stopper()
        self.clock.advance(pool.IDLE_SECONDS)
        for tags in (OSError('metadata unavailable'), dict(JudgeHoldUntil='soon', JudgeMaintenanceHoldUntil=None),
                     dict(JudgeHoldUntil='2026-01-01T00:00:00', JudgeMaintenanceHoldUntil=None)):
            with self.subTest(tags=tags):
                self.tags = tags
                self.assertFalse(stopper.due())
                self.clock.advance(pool.CHECK_SECONDS)
        failures = [json.loads(line) for line in self.output.getvalue().splitlines()]
        self.assertEqual([f['reason'] for f in failures], ['pool_hold_unavailable'])

    def test_stop_logs_before_power_off(self):
        with patch.object(pool.time, 'sleep'), patch.object(pool.subprocess, 'run') as run:
            self.stopper().stop()
        run.assert_called_once_with(['systemctl', 'poweroff', '--no-block'], check=True)
        self.assertEqual(json.loads(self.output.getvalue())['event'], 'idle_stop')


class MetadataTests(unittest.TestCase):
    def serve(self, tags, status=None):
        requests = []

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_PUT(self):
                requests.append(('PUT', self.path, self.headers.get('X-aws-ec2-metadata-token-ttl-seconds')))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'token')

            def do_GET(self):
                requests.append(('GET', self.path, self.headers.get('X-aws-ec2-metadata-token')))
                key = self.path.rsplit('/', 1)[1]
                code = status or (200 if key in tags else 404)
                self.send_response(code)
                self.end_headers()
                if code == 200:
                    self.wfile.write(tags[key].encode())

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.shutdown)
        return 'http://127.0.0.1:%d' % server.server_address[1], requests

    def test_tags_use_imdsv2_and_missing_tags_are_none(self):
        endpoint, requests = self.serve({'JudgeHoldUntil': '2026-01-01T00:00:00Z'})
        self.assertEqual(pool.instance_tags(endpoint, pool.HOLD_TAGS),
                         {'JudgeHoldUntil': '2026-01-01T00:00:00Z', 'JudgeMaintenanceHoldUntil': None})
        self.assertEqual(requests[0], ('PUT', '/latest/api/token', '60'))
        self.assertTrue(all(r[2] == 'token' for r in requests[1:]))

    def test_metadata_errors_are_not_missing_tags(self):
        endpoint, _ = self.serve({}, status=500)
        with self.assertRaises(Exception):
            pool.instance_tags(endpoint, pool.HOLD_TAGS)


if __name__ == '__main__':
    unittest.main()
