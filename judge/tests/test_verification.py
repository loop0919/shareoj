import copy
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import verify
from admission import publication


class VerificationTests(unittest.TestCase):
    def setUp(self):
        self.receipt = dict(phase='smoke', region='ap-northeast-1', runId='a' * 32,
                            instances=['mi-a', 'mi-b'], commands={'mi-a': 'cmd-a', 'mi-b': 'cmd-b'})
        self.report = dict(runId='a' * 32, runtimeDigest='sha256:' + 'b' * 64,
                           passedRuntimes=['cpp17-isolate'], failedRuntimes=[])

    def test_missing_failed_stale_or_different_host_blocks_publication(self):
        cases = [dict(runtimeDigest='sha256:' + 'c' * 64), dict(runId='c' * 32),
                 dict(failedRuntimes=['cpp17-isolate']), dict(passedRuntimes=[])]
        for change in cases:
            with self.subTest(change=change), self.assertRaises(ValueError):
                verify.merge_reports(self.receipt, {'mi-a': self.report, 'mi-b': {**self.report, **change}})
        with self.assertRaises(ValueError):
            verify.merge_reports(self.receipt, {'mi-a': self.report})
        combined = verify.merge_reports(self.receipt, dict.fromkeys(self.receipt['instances'], self.report))
        with self.assertRaises(ValueError):
            publication(combined, 'cpp17')

    def test_only_all_ready_hosts_can_be_published(self):
        self.receipt['phase'] = 'start'
        reports = {node: {**self.report, 'ready': True} for node in self.receipt['instances']}
        result = verify.merge_reports(self.receipt, reports)
        self.assertEqual(publication(result, 'cpp17')[0], self.report['runtimeDigest'])
        result['hosts']['mi-b']['ready'] = False
        with self.assertRaises(ValueError):
            publication(result, 'cpp17')
        with self.assertRaises(ValueError):
            verify.merge_reports(self.receipt, reports)

    def test_collect_pending_and_terminal_failure_never_creates_pass_report(self):
        for status, expected in [('InProgress', 3), ('Failed', None), ('TimedOut', None)]:
            with self.subTest(status=status), tempfile.TemporaryDirectory() as name:
                directory = Path(name)
                with patch.object(verify, 'aws', return_value={'CommandInvocations': [{'Status': status}]}):
                    if expected:
                        self.assertEqual(verify.collect(directory, self.receipt), expected)
                    else:
                        with self.assertRaises(RuntimeError):
                            verify.collect(directory, self.receipt)
                self.assertFalse((directory / 'report.json').exists())
                self.assertNotEqual(json.loads((directory / 'status.json').read_text())['status'], 'passed')

    def test_collect_uses_separate_stdout_and_rejects_digest_disagreement(self):
        def fake_aws(region, service, operation, *args):
            if operation == 'list-command-invocations':
                return {'CommandInvocations': [{'Status': 'Success'}]}
            report = copy.deepcopy(self.report)
            if '--instance-id' in args and args[args.index('--instance-id') + 1] == 'mi-b':
                report['runtimeDigest'] = 'sha256:' + 'c' * 64
            return {'StandardOutputContent': json.dumps(report), 'StandardErrorContent': 'diagnostic'}
        with tempfile.TemporaryDirectory() as name, patch.object(verify, 'aws', side_effect=fake_aws):
            with self.assertRaises(ValueError):
                verify.collect(Path(name), self.receipt)
            self.assertEqual(json.loads(Path(name, 'status.json').read_text())['status'], 'failed')

    def test_new_run_accepts_ec2_and_hybrid_nodes(self):
        with tempfile.TemporaryDirectory() as name, patch.object(verify, 'aws', return_value={'Account': '1'}):
            nodes = ['i-0123456789abcdef0', 'i-0123abcd', 'mi-0a1b']
            self.assertEqual(verify.new_run(Path(name) / 'ok', 'test', nodes, 'smoke')['instances'], nodes)
            for invalid in ([], ['i-0a1b2c3d', 'i-0a1b2c3d'], ['i-XYZ12345'], ['i-0a1b'], ['vol-0123456789abcdef0']):
                with self.subTest(invalid=invalid), self.assertRaises(ValueError):
                    verify.new_run(Path(name) / 'bad', 'test', invalid, 'smoke')

    def test_submission_receipt_is_saved_before_collection(self):
        with tempfile.TemporaryDirectory() as name, patch.object(verify, 'aws', return_value={'Command': {'CommandId': 'id'}}):
            receipt = {**self.receipt, 'commands': {}}
            verify.submit(Path(name), receipt, 'mi-a', 'true')
            self.assertEqual(json.loads(Path(name, 'receipt.json').read_text())['commands'], {'mi-a': 'id'})
            with self.assertRaises(ValueError):
                verify.collect(Path(name), receipt)

    def test_remote_commands_parse_without_running_them(self):
        for script in (verify.smoke_command('a' * 32), verify.start_command('a' * 32, 'sha256:' + 'b' * 64)):
            subprocess.run(['bash', '-n'], input=script, text=True, check=True)
            python = script.split("<<'PY'\n", 1)[1].rsplit('\nPY\n', 1)[0]
            compile(python, '<remote verification>', 'exec')
