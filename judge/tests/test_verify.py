import json
from pathlib import Path
import sys
from types import SimpleNamespace
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import verify

DIGEST = 'sha256:' + 'a' * 64
URL = 'https://sqs.ap-northeast-1.amazonaws.com/123456789012/judge-requests-a'


class StartCommandTests(unittest.TestCase):
    def test_start_command_rewrites_the_request_queue(self):
        script = verify.start_command('r', DIGEST, URL)
        self.assertIn('JUDGE_REQUEST_QUEUE_URL=', script)
        self.assertIn(URL, script)
        self.assertIn('expected exactly one request queue setting', script)
        plain = verify.start_command('r', DIGEST)
        self.assertNotIn('JUDGE_REQUEST_QUEUE_URL=', plain)
        self.assertNotIn('request queue', plain)

    def test_start_command_rejects_other_urls(self):
        for value in ['https://example.com/x', URL + '; reboot', URL + '\n', "'$(id)'",
                      'http://sqs.ap-northeast-1.amazonaws.com/123456789012/q', '']:
            with self.subTest(value=value), self.assertRaises(ValueError):
                verify.start_command('r', DIGEST, value)

    def start(self, *events):
        """Run the host's start script against a journal that holds the events; returns what it printed."""
        script = verify.start_command('r', DIGEST)
        code = compile(script.split("<<'PY'\n", 1)[1].rsplit('\nPY\n', 1)[0], '<start worker>', 'exec')
        report = dict(runId='r', runtimeDigest=DIGEST, failedRuntimes=[])
        journal = '\n'.join(json.dumps(event) for event in events)

        def read_text(path):
            return json.dumps(report) if path.name == 'report.json' else 'JUDGE_RUNTIME_DIGEST=old\n'

        def run(args, **kwargs):
            # The first is-active asks whether a worker already runs; the checked calls succeed.
            return SimpleNamespace(returncode=0 if kwargs.get('check') else 3)
        host = type(sys)('host')
        host.verify_assets = lambda: DIGEST
        with patch.dict(sys.modules, host=host), patch.object(sys, 'argv', ['-', 'r', DIGEST]), \
                patch.object(Path, 'read_text', autospec=True, side_effect=read_text), patch.object(Path, 'write_text'), \
                patch.object(Path, 'chmod'), patch('subprocess.run', side_effect=run), patch('time.sleep'), \
                patch('subprocess.check_output', side_effect=lambda args, text=False: journal if args[0] == 'journalctl' else 'abc\n'), \
                patch('builtins.print') as printed:
            exec(code, {'__name__': '__main__'})
        return json.loads(printed.call_args.args[0])

    def test_start_is_ready_despite_a_failing_checker(self):
        # A rolling update starts workers on the lane in use, so a problem author's failing checker can come first.
        report = self.start(dict(event='failure', category='judge_code', reason='checker_execution_failed'),
                            dict(event='worker_started'))
        self.assertTrue(report['ready'])

    def test_start_fails_on_a_worker_failure(self):
        for category in ('platform', 'unknown'):
            with self.subTest(category=category), self.assertRaisesRegex(SystemExit, 'worker reported a failure'):
                self.start(dict(event='failure', category=category, reason='x'), dict(event='worker_started'))


if __name__ == '__main__':
    unittest.main()
