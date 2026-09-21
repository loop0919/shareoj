import importlib.util
import json
from pathlib import Path
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
spec = importlib.util.spec_from_file_location('api_smoke', ROOT / 'smoke-api.py')
smoke = importlib.util.module_from_spec(spec)
spec.loader.exec_module(smoke)


class APISmokeTests(unittest.TestCase):
    def test_four_cases_and_cleanup_run_without_live_aws(self):
        submissions, deleted = {}, []

        def request(base, method, path, data=None, token=''):
            if method == 'PUT' and path.startswith('/my/problems/'):
                self.assertEqual(data['draft']['memoryLimitMb'], '315')
            if path == '/runtimes':
                return dict(maintenance=False, items=[dict(id='python314')])
            if path == '/auth/login':
                return dict(access_token='secret-test-token')
            if path == '/my/submissions':
                expected = ['TLE', 'AC', 'TLE', 'AC'] if data['easyTest'] else \
                    ['TLE', 'AC', 'AC', 'AC'] if 'n == 1' in data['source'] else ['TLE', 'AC', 'TLE', 'SKIPPED']
                id = str(len(submissions))
                submissions[id] = dict(verdict='TLE', passed=expected.count('AC'), total=4,
                                       cases=[dict(verdict=v) for v in expected])
                return dict(id=id)
            if path.startswith('/my/submissions/'):
                return dict(status='DONE', result=submissions[path.rsplit('/', 1)[1]])
            if method == 'DELETE':
                deleted.append(path)
            return dict(version=1)

        with tempfile.TemporaryDirectory() as name:
            report = Path(name, 'report.json')
            argv = ['smoke-api.py', '--function', 'test-api', '--api-url', 'https://test.invalid', '--report', str(report)]
            with patch.object(sys, 'argv', argv), patch.object(smoke, 'request', side_effect=request), \
                    patch.object(smoke, 'aws', return_value={'Environment': {'Variables': {'COGNITO_USER_POOL_ID': 'pool'}}}), \
                    patch.object(smoke, 'cognito') as cognito:
                smoke.main()
            saved = json.loads(report.read_text())
            self.assertEqual(saved['status'], 'passed')
            self.assertEqual(len(saved['results']), 4)
            self.assertEqual(len(deleted), 2)
            self.assertEqual(json.loads(report.with_suffix('.cleanup.json').read_text())['users'], [])
            creates = [c.args[2] for c in cognito.call_args_list if c.args[1] == 'admin-create-user']
            self.assertEqual(len(creates), 2)
            self.assertTrue(all(c['MessageAction'] == 'SUPPRESS' for c in creates))
            self.assertNotIn('secret-test-token', report.read_text())

    def test_cleanup_continues_after_one_user_fails_and_records_remaining_work(self):
        args = SimpleNamespace(region='test', api_url='https://test.invalid')
        users = [dict(username='first', problem=None), dict(username='second', problem=None)]
        with tempfile.TemporaryDirectory() as name, \
                patch.object(smoke, 'cognito', side_effect=[RuntimeError('retry'), {}]) as cognito:
            path = Path(name, 'cleanup.json')
            with self.assertRaises(RuntimeError):
                smoke.cleanup(args, 'pool', users, path)
            self.assertEqual(cognito.call_count, 2)
            self.assertEqual(json.loads(path.read_text())['users'], [users[0]])

    def test_wrong_knockout_result_and_fake_resource_metrics_fail(self):
        result = dict(verdict='TLE', passed=1, total=4, cases=[dict(verdict=v) for v in ['TLE', 'AC', 'TLE', 'SKIPPED']])
        expected = ['TLE', 'AC', 'TLE', 'SKIPPED']
        smoke.assert_result(result, expected)
        result['cases'][-1]['cpuTimeMs'] = 0
        with self.assertRaises(ValueError):
            smoke.assert_result(result, expected)

    def test_parallel_evidence_requires_overlapping_intervals(self):
        args = SimpleNamespace(region='test', instance=['mi-a', 'mi-b'])
        for offset in (1, 3):
            def fake_aws(region, service, operation, *options):
                if operation == 'send-command':
                    return {'Command': {'CommandId': 'command'}}
                if operation == 'list-command-invocations':
                    return {'CommandInvocations': [{'Status': 'Success'}]}
                node = options[options.index('--instance-id') + 1]
                start = 0 if node == 'mi-a' else offset
                events = [dict(submissionId=node, event=event, timestamp=f'2026-01-01T00:00:0{second}+00:00')
                          for event, second in [('judge_started', start), ('judge_finished', start + 2)]]
                return {'StandardOutputContent': '\n'.join(json.dumps(e) for e in events)}
            with self.subTest(offset=offset), patch.object(smoke, 'aws', side_effect=fake_aws), patch.object(smoke.time, 'sleep'):
                if offset == 1:
                    self.assertEqual(set(smoke.parallel_check(args, args.instance)), set(args.instance))
                else:
                    with self.assertRaises(ValueError):
                        smoke.parallel_check(args, args.instance)
