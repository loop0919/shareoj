import base64
import hashlib
import io
import json
import subprocess
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import rollout


class RolloutTests(unittest.TestCase):
    def test_legacy_bridge_null_is_not_an_empty_database(self):
        def invoke(region, *args):
            Path(args[-1]).write_text('null')
            return {}
        with patch.object(rollout, 'aws', side_effect=invoke), self.assertRaises(ValueError):
            rollout.database_status(dict(region='test', bridge='bridge'))

    def test_empty_queue_does_not_hide_pending_database_rows(self):
        attributes = dict.fromkeys(('ApproximateNumberOfMessages', 'ApproximateNumberOfMessagesNotVisible', 'ApproximateNumberOfMessagesDelayed'), '0')
        statuses = [dict(pending=1, undispatched=1)] + [dict(pending=0, undispatched=0)] * 3
        with patch.object(rollout, 'database_status', side_effect=statuses) as status, \
                patch.object(rollout, 'aws', return_value=dict(Attributes=attributes)), patch.object(rollout.time, 'sleep'):
            rollout.wait_empty(dict(region='test', queues=['queue']))
            self.assertEqual(status.call_count, 4)

    def test_prepare_drains_before_stopping_workers(self):
        events = []
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            package = directory / 'bridge.zip'
            package.write_bytes(b'package')
            digest = base64.b64encode(hashlib.sha256(b'package').digest()).decode()
            config = dict(region='test', api='api', bridge='bridge', bridge_package=str(package), nodes=['mi-a', 'mi-b'], worker_alarms=['alarm'])
            def aws(region, service, operation, *args):
                if operation == 'get-function-configuration':
                    return dict(CodeSha256=digest, Timeout=0)
                if operation == 'describe-alarms':
                    return dict(MetricAlarms=[dict(AlarmName='alarm', ActionsEnabled=True)])
                return {}
            receipt = dict(runId='a' * 32, instances=config['nodes'], commands={})
            with patch.object(rollout, 'preflight'), patch.object(rollout, 'backup_lambda'), \
                    patch.object(rollout, 'aws', side_effect=aws), patch.object(rollout, 'database_status'), \
                    patch.object(rollout, 'update_env', side_effect=lambda *a: events.append('pause')), \
                    patch.object(rollout, 'wait_empty', side_effect=lambda *a: events.append('drain')), \
                    patch.object(rollout, 'delivery', side_effect=lambda *a: events.append('disable')), \
                    patch.object(rollout.time, 'sleep'), patch.object(rollout, 'new_run', return_value=receipt), \
                    patch.object(rollout, 'submit', side_effect=lambda *a: events.append('stop')), \
                    patch.object(rollout, 'collect', return_value=0):
                state = {}
                rollout.prepare(config, directory, state)
            self.assertEqual(events, ['pause', 'drain', 'disable', 'drain', 'stop', 'stop'])
            self.assertTrue(state['prepared'])
            self.assertEqual(state['alarm_actions'], {'alarm': True})

    def test_settings_failure_never_enables_delivery_or_publishes(self):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            digest = 'sha256:' + 'a' * 64
            host = dict(runtimeDigest=digest, passedRuntimes=['python314-isolate'], failedRuntimes=[], ready=True, runId='b' * 32)
            report = {**host, 'hosts': {'mi-a': host, 'mi-b': host}}
            report_path = directory / 'report.json'
            report_path.write_text(json.dumps(report))
            config = dict(region='test', api='api', bridge='bridge', nodes=['mi-a', 'mi-b'], runtimes=['python314'])
            with patch.object(rollout, 'update_env') as update, patch.object(rollout, 'wait_empty'), \
                    patch.object(rollout, 'aws', return_value=dict(Timeout=0)), patch.object(rollout.time, 'sleep'), \
                    patch.object(rollout, 'healthy_workers'), patch.object(rollout, 'database_status', return_value=dict(runtimeDigest=digest)), \
                    patch.object(rollout, 'sync_settings', side_effect=RuntimeError('denied')), patch.object(rollout, 'delivery') as delivery:
                with self.assertRaises(RuntimeError):
                    rollout.finish(config, directory, dict(prepared=True), report_path)
            delivery.assert_not_called()
            updates = [c.args[2] for c in update.call_args_list]
            self.assertFalse(any(v.get('JUDGE_ENABLED_RUNTIMES') == 'python314' for v in updates))

    def test_failure_repauses_admission_and_records_recovery_state(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            package = root / 'package'
            package.write_bytes(b'fixed')
            config = dict(account='123', region='test', api='api', release=str(package), bridge_package=str(package))
            config_path = root / 'config.json'
            config_path.write_text(json.dumps(config))
            directory = root / 'run'
            directory.mkdir()
            (directory / 'state.json').write_text(json.dumps(dict(prepared=True, maintenance=True)))
            argv = ['rollout.py', 'finish', '--config', str(config_path), '--run-dir', str(directory)]
            with patch.object(sys, 'argv', argv), patch.object(rollout, 'ROOT', root), patch.object(rollout.os, 'chdir'), \
                    patch.object(rollout, 'LOG_DIRECTORY', None), patch.object(rollout, 'aws', return_value=dict(Account='123')), \
                    patch.object(rollout, 'update_env') as update:
                with self.assertRaises(ValueError):
                    rollout.main()  # Missing --report is a failure, never an implicit publication.
            self.assertEqual(update.call_args.args[2]['JUDGE_ENABLED_RUNTIMES'], 'none')
            state = json.loads((directory / 'state.json').read_text())
            self.assertEqual(state['status'], 'failed')
            self.assertTrue(state['admissionPaused'])

    def test_finish_reaches_complete_only_after_api_smoke_and_state_refresh(self):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            digest = 'sha256:' + 'a' * 64
            host = dict(runtimeDigest=digest, passedRuntimes=['python314-isolate'], failedRuntimes=[], ready=True, runId='b' * 32)
            report_path = directory / 'report.json'
            report_path.write_text(json.dumps({**host, 'hosts': {'mi-a': host, 'mi-b': host}}))
            config = dict(region='test', api='api', bridge='bridge', nodes=['mi-a', 'mi-b'], runtimes=['python314'],
                          smoke_runtime='python314', api_url='https://test.invalid')
            calls = []
            def command(*args):
                calls.append(args)
                if 'judge/smoke-api.py' in args:
                    Path(args[args.index('--report') + 1]).write_text('{"status":"passed"}')
                return ''
            catalog = io.BytesIO(b'{"maintenance":false,"items":[{"id":"python314"}]}')
            with patch.object(rollout, 'update_env'), patch.object(rollout, 'wait_empty'), \
                    patch.object(rollout, 'aws', return_value=dict(Timeout=0)), patch.object(rollout.time, 'sleep'), \
                    patch.object(rollout, 'healthy_workers'), patch.object(rollout, 'database_status', return_value=dict(runtimeDigest=digest)), \
                    patch.object(rollout, 'sync_settings'), patch.object(rollout, 'delivery'), \
                    patch.object(rollout.urllib.request, 'urlopen', return_value=catalog), patch.object(rollout, 'run', side_effect=command):
                state = dict(prepared=True, alarm_actions={'enabled': True, 'disabled': False})
                rollout.finish(config, directory, state, report_path)
            self.assertEqual(state['status'], 'passed')
            self.assertEqual(state['step'], 'complete')
            self.assertFalse(state['maintenance'])
            self.assertFalse(state['admissionPaused'])
            plans = [args for args in calls if args[0] == 'terraform' and 'plan' in args]
            self.assertEqual(len(plans), 2)
            self.assertTrue(all('-refresh-only' in args for args in plans))

    def test_release_cleanup_keeps_only_published_worker_version(self):
        release = 'a' * 64
        active_key = f'releases/{release}/worker.tar.gz'
        old_key = f'releases/{"b" * 64}/worker.tar.gz'
        digest = 'sha256:' + 'c' * 64
        config = dict(region='test', account='123', api='api', bridge='bridge', bucket='bucket')
        state = dict(status='passed', step='complete', releaseSHA256=release, runtimeDigest=digest)
        versions = [
            dict(Key=active_key, VersionId='active', IsLatest=True, Size=10, LastModified='2026-09-21'),
            dict(Key=active_key, VersionId='duplicate', IsLatest=False, Size=10, LastModified='2026-09-20'),
            dict(Key=old_key, VersionId='old', IsLatest=True, Size=20, LastModified='2026-09-19'),
            dict(Key='releases/bridge.zip', VersionId='bridge', IsLatest=True, Size=30, LastModified='2026-09-18'),
        ]
        deleted = []
        def aws(region, service, operation, *args):
            if service == 'sts':
                return dict(Account='123')
            if service == 'lambda':
                name = args[-1]
                variable = 'JUDGE_CPP_IMAGE' if name == 'api' else 'JUDGE_RUNTIME_DIGEST'
                return dict(Environment=dict(Variables={variable: digest}))
            if operation == 'head-object':
                return dict(VersionId='active')
            if operation == 'list-object-versions':
                return dict(Versions=versions)
            if operation == 'delete-objects':
                deleted.extend(json.loads(args[-1])['Objects'])
                return {}
            raise AssertionError(operation)
        with patch.object(rollout, 'aws', side_effect=aws):
            self.assertEqual(rollout.prune_worker_releases(config, state), (2, 30))
        self.assertEqual(deleted, [dict(Key=active_key, VersionId='duplicate'), dict(Key=old_key, VersionId='old')])

    def test_release_cleanup_refuses_newer_upload(self):
        release = 'a' * 64
        key = f'releases/{release}/worker.tar.gz'
        config = dict(region='test', account='123', api='api', bridge='bridge', bucket='bucket')
        digest = 'sha256:' + 'c' * 64
        state = dict(status='passed', step='complete', releaseSHA256=release, runtimeDigest=digest)
        def aws(region, service, operation, *args):
            if service == 'sts':
                return dict(Account='123')
            if service == 'lambda':
                variable = 'JUDGE_CPP_IMAGE' if args[-1] == 'api' else 'JUDGE_RUNTIME_DIGEST'
                return dict(Environment=dict(Variables={variable: digest}))
            if operation == 'head-object':
                return dict(VersionId='active')
            if operation == 'list-object-versions':
                return dict(Versions=[
                    dict(Key=key, VersionId='active', IsLatest=True, LastModified='2026-09-21'),
                    dict(Key=f'releases/{"b" * 64}/worker.tar.gz', VersionId='newer',
                         IsLatest=True, LastModified='2026-09-22')])
            raise AssertionError('deletion must not be attempted')
        with patch.object(rollout, 'aws', side_effect=aws), self.assertRaises(ValueError):
            rollout.prune_worker_releases(config, state)

    def test_release_cleanup_error_keeps_successful_rollout(self):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            state = dict(status='passed', step='complete')
            with patch.object(rollout, 'prune_worker_releases', side_effect=RuntimeError('temporary AWS error')):
                rollout.cleanup_after_success({}, directory, state)
            saved = json.loads((directory / 'state.json').read_text())
            self.assertEqual(saved['status'], 'passed')
            self.assertEqual(saved['releasePruneError'], 'temporary AWS error')
            self.assertNotIn('releasePruned', saved)

    def test_settings_sync_checks_github_and_keeps_previous_local_values(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            for path in ('infra/api', 'infra/judge', 'run'):
                (root / path).mkdir(parents=True)
            previous = root / 'infra/judge/zz-rollout.auto.tfvars.json'
            previous.write_text('{"runtime_digest":"old"}')
            values = {}
            def command(*args):
                if args[2] == 'set':
                    values[args[3]] = args[args.index('--body') + 1]
                    return ''
                return json.dumps([dict(name=name, value=value) for name, value in values.items()])
            config = dict(repository='owner/repo', github_environment='dev', nodes=['mi-a', 'mi-b'])
            with patch.object(rollout, 'ROOT', root), patch.object(rollout, 'run', side_effect=command):
                rollout.sync_settings(config, root / 'run', 'sha256:fixed', ['python314'])
            self.assertEqual(json.loads(previous.read_text())['worker_count'], 2)
            backup = json.loads((root / 'run/infra-judge-variables.json').read_text())
            self.assertEqual(json.loads(backup['content'])['runtime_digest'], 'old')

    def test_fresh_worker_health_command_parses_without_touching_hosts(self):
        with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'new_run', return_value={}), \
                patch.object(rollout, 'submit') as submit, patch.object(rollout, 'collect', return_value=0):
            rollout.healthy_workers(dict(region='test', nodes=['mi-a', 'mi-b']), Path(name), 'sha256:' + 'a' * 64)
            self.assertEqual(submit.call_count, 2)
            script = submit.call_args.args[-1]
            subprocess.run(['bash', '-n'], input=script, text=True, check=True)
            compile(script.split("<<'PY'\n", 1)[1].rsplit('\nPY\n', 1)[0], '<worker health>', 'exec')
