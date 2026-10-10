import base64
import datetime
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

    def test_prepare_also_stops_retired_hosts(self):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            package = directory / 'bridge.zip'
            package.write_bytes(b'package')
            digest = base64.b64encode(hashlib.sha256(b'package').digest()).decode()
            config = dict(region='test', api='api', bridge='bridge', bridge_package=str(package),
                          nodes=['i-0123456789abcdef0'], retire_nodes=['mi-a', 'mi-b'], worker_alarms=['alarm'])
            def aws(region, service, operation, *args):
                if operation == 'get-function-configuration':
                    return dict(CodeSha256=digest, Timeout=0)
                if operation == 'describe-alarms':
                    return dict(MetricAlarms=[dict(AlarmName='alarm', ActionsEnabled=True)])
                return {}
            stopped = []
            with patch.object(rollout, 'preflight'), patch.object(rollout, 'backup_lambda'), \
                    patch.object(rollout, 'aws', side_effect=aws), patch.object(rollout, 'database_status'), \
                    patch.object(rollout, 'update_env'), patch.object(rollout, 'wait_empty'), patch.object(rollout, 'delivery'), \
                    patch.object(rollout.time, 'sleep'), \
                    patch.object(rollout, 'new_run', side_effect=lambda d, r, nodes, p: dict(runId='a' * 32, instances=nodes, commands={})), \
                    patch.object(rollout, 'submit', side_effect=lambda d, receipt, node, script: stopped.append(node)), \
                    patch.object(rollout, 'collect', return_value=0):
                rollout.prepare(config, directory, {})
            self.assertEqual(stopped, ['i-0123456789abcdef0', 'mi-a', 'mi-b'])

    def test_enabled_retired_worker_blocks_finish(self):
        commands = {}
        with tempfile.TemporaryDirectory() as name, \
                patch.object(rollout, 'new_run', side_effect=lambda d, r, nodes, p: dict(runId='a' * 32, instances=nodes, commands={})), \
                patch.object(rollout, 'submit', side_effect=lambda d, receipt, node, script: commands.setdefault(node, script)), \
                patch.object(rollout, 'collect', return_value=1):
            rollout.retired_workers_stopped(dict(region='test'), Path(name))
            with self.assertRaises(RuntimeError):
                rollout.retired_workers_stopped(dict(region='test', retire_nodes=['mi-a']), Path(name))
        self.assertIn('systemctl is-enabled', commands['mi-a'])
        subprocess.run(['bash', '-n', '-c', commands['mi-a']], check=True)

    def pool_aws(self, states, calls):
        def aws(region, service, operation, *args):
            calls.append((operation, *args))
            if operation == 'describe-instances':
                return dict(Reservations=[dict(Instances=[dict(InstanceId=node, State=dict(Name=state))]) for node, state in states.items()])
            if operation == 'describe-instance-information':
                return dict(InstanceInformationList=[dict(PingStatus='Online')] * len(states))
            return {}
        return aws

    def test_pool_must_match_deployment_targets(self):
        calls = []
        config = dict(region='test', pool='judge-dev', nodes=['i-0123456789abcdef0', 'i-0123456789abcdef1'])
        with patch.object(rollout, 'aws', side_effect=self.pool_aws({'i-0123456789abcdef0': 'running'}, calls)), \
                patch.object(rollout, 'run') as run, self.assertRaises(ValueError):
            rollout.ensure_pool(config)
        self.assertEqual([c[0] for c in calls], ['describe-instances'])
        run.assert_not_called()

    def test_pool_hosts_are_held_started_and_online_before_maintenance(self):
        calls, waits = [], []
        states = {'i-0123456789abcdef0': 'running', 'i-0123456789abcdef1': 'stopped', 'i-0123456789abcdef2': 'stopping'}
        config = dict(region='test', pool='judge-dev', nodes=list(states))
        with patch.object(rollout, 'aws', side_effect=self.pool_aws(states, calls)), \
                patch.object(rollout, 'run', side_effect=lambda *args: waits.append(args[3]) or calls.append(('wait', args[3]))):
            rollout.ensure_pool(config)
        self.assertEqual([c[0] for c in calls], ['describe-instances', 'create-tags', 'wait', 'start-instances', 'wait', 'describe-instance-information'])
        self.assertTrue(calls[1][-1].startswith('Key=JudgeMaintenanceHoldUntil,Value=20'))
        self.assertEqual(calls[3][2:], ('i-0123456789abcdef1', 'i-0123456789abcdef2'))
        self.assertEqual(waits, ['instance-stopped', 'instance-running'])
        with patch.object(rollout, 'aws') as aws:
            rollout.ensure_pool(dict(region='test', nodes=['mi-a']))
        aws.assert_not_called()

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
            updates = [c.args[2] for c in update.call_args_list if c.args[1] == config['api']]
            update.assert_any_call('test', 'bridge', dict(JUDGE_RUNTIME_DIGEST=digest, JUDGE_ENABLED_RUNTIMES='python314'))
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

    def run_finish(self, **extra):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            digest = 'sha256:' + 'a' * 64
            host = dict(runtimeDigest=digest, passedRuntimes=['python314-isolate'], failedRuntimes=[], ready=True, runId='b' * 32)
            report_path = directory / 'report.json'
            nodes = ['i-0123456789abcdef0', 'i-0123456789abcdef1', 'i-0123456789abcdef2']
            report_path.write_text(json.dumps({**host, 'hosts': dict.fromkeys(nodes, host)}))
            config = dict(region='test', api='api', bridge='bridge', nodes=nodes, runtimes=['python314'],
                          smoke_runtime='python314', api_url='https://test.invalid', **extra)
            calls, events = [], []
            def command(*args):
                calls.append(args)
                if 'judge/smoke-api.py' in args:
                    Path(args[args.index('--report') + 1]).write_text('{"status":"passed"}')
                return ''
            def aws(region, service, operation, *args):
                events.append((operation, *args))
                return dict(Timeout=0)
            catalog = io.BytesIO(b'{"maintenance":false,"items":[{"id":"python314"}]}')
            with patch.object(rollout, 'update_env'), patch.object(rollout, 'wait_empty'), \
                    patch.object(rollout, 'aws', side_effect=aws), patch.object(rollout.time, 'sleep'), \
                    patch.object(rollout, 'healthy_workers', side_effect=lambda *a: events.append(('healthy',))), \
                    patch.object(rollout, 'database_status', return_value=dict(runtimeDigest=digest)), \
                    patch.object(rollout, 'sync_settings'), patch.object(rollout, 'delivery', side_effect=lambda *a: events.append(('delivery',))), \
                    patch.object(rollout.urllib.request, 'urlopen', return_value=catalog), patch.object(rollout, 'run', side_effect=command):
                state = dict(prepared=True, alarm_actions={'enabled': True, 'disabled': False})
                rollout.finish(config, directory, state, report_path)
            return state, calls, events, nodes, digest

    def test_finish_reaches_complete_only_after_api_smoke_and_state_refresh(self):
        state, calls, events, nodes, _ = self.run_finish()
        self.assertEqual(state['status'], 'passed')
        self.assertEqual(state['step'], 'complete')
        self.assertFalse(state['maintenance'])
        self.assertFalse(state['admissionPaused'])
        self.assertFalse(any(e[0] == 'create-tags' for e in events))
        smoke = next(args for args in calls if 'judge/smoke-api.py' in args)
        self.assertEqual([smoke[i + 1] for i, arg in enumerate(smoke) if arg == '--instance'], nodes)
        plans = [args for args in calls if args[0] == 'terraform' and 'plan' in args]
        self.assertEqual(len(plans), 2)
        self.assertTrue(all('-refresh-only' in args for args in plans))

    def test_finish_tags_the_pool_after_health_and_releases_the_hold(self):
        state, _, events, nodes, digest = self.run_finish(pool='judge-dev')
        names = [e[0] for e in events]
        tags = [e for e in events if e[0] == 'create-tags']
        self.assertEqual(tags[0], ('create-tags', '--resources', *nodes, '--tags', 'Key=JudgeInstalledDigest,Value=' + digest))
        self.assertLess(names.index('healthy'), names.index('create-tags'))
        self.assertLess(names.index('create-tags'), names.index('delivery'))
        self.assertTrue(tags[1][-1].startswith('Key=JudgeMaintenanceHoldUntil,Value=20'))
        self.assertTrue(state['maintenanceHoldReleased'])

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
            state = dict(status='passed', step='complete', releaseSHA256='a' * 64)
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
            previous.write_text('{"runtime_digest":"old","worker_count":2}')
            values = {}
            def command(*args):
                if args[2] == 'set':
                    values[args[3]] = args[args.index('--body') + 1]
                    return ''
                return json.dumps([dict(name=name, value=value) for name, value in values.items()])
            config = dict(repository='owner/repo', github_environment='dev', nodes=['i-0a', 'i-0b', 'i-0c'])
            with patch.object(rollout, 'ROOT', root), patch.object(rollout, 'run', side_effect=command):
                rollout.sync_settings(config, root / 'run', 'sha256:fixed', ['python314'])
                # A resumed sync merges into the saved original, not into its own earlier output.
                rollout.sync_settings(config, root / 'run', 'sha256:fixed', ['python314'])
            self.assertEqual(json.loads(previous.read_text()),
                             dict(runtime_digest='sha256:fixed', enabled=True, enabled_runtimes=['python314'], worker_count=2))
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


class RollingTests(unittest.TestCase):
    first, primary, other = 'i-0123456789abcdef1', 'i-0123456789abcdef0', 'i-0123456789abcdef2'
    old, new = 'sha256:' + 'a' * 64, 'sha256:' + 'b' * 64

    def run_rolling(self, state=None, drain=None, **config):
        events = []
        config = dict(region='test', api='api', bridge='bridge', pool='judge-dev', nodes=[self.primary, self.first, self.other],
                      worker_alarms=['alarm'], runtimes=['python314'], queues=['requests'], **config)
        state = {} if state is None else state

        def aws(region, service, operation, *args):
            if operation == 'get-function-configuration':
                variables = dict(JUDGE_RUNTIME_DIGEST=self.old, JUDGE_CPP_IMAGE=self.old, JUDGE_ENABLED_RUNTIMES='python314')
                return dict(Environment=dict(Variables=variables), Timeout=0)
            if operation == 'describe-alarms':
                return dict(MetricAlarms=[dict(AlarmName='alarm', ActionsEnabled=True)])
            events.append((operation, *args))
            return {}

        def prepare_hosts(config, directory, state, nodes, label, expected_digest=None):
            events.append(('prepare', label, tuple(nodes), expected_digest))
            verification = directory / ('verify-' + label)
            verification.mkdir(exist_ok=True)
            (verification / 'report.json').write_text(json.dumps(dict(runtimeDigest=self.new)))
            state.setdefault('packages', 'c' * 64)
            return verification

        def status():
            paused = events and any(e[0] == 'dispatch' for e in events) and [e for e in events if e[0] == 'dispatch'][-1][1]
            switched = any(e[:2] == ('env', 'bridge') for e in events)
            return dict(runtimeDigest=self.new if switched else self.old, previousRuntimeDigest=self.old if switched else '',
                        dispatchPaused=bool(paused), pending=0, undispatched=0)

        with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'preflight'), \
                patch.object(rollout, 'pool_bursts', return_value=[self.first, self.other]), \
                patch.object(rollout, 'deploy_bridge', side_effect=lambda *a, **k: events.append(('bridge-code',))), \
                patch.object(rollout, 'contest_guard', side_effect=lambda c: events.append(('contest',))), \
                patch.object(rollout, 'aws', side_effect=aws), patch.object(rollout.time, 'sleep'), \
                patch.object(rollout, 'ssm_step', side_effect=lambda d, step, c, nodes, script: events.append(('ssm', step, tuple(nodes)))), \
                patch.object(rollout, 'prepare_hosts', side_effect=prepare_hosts), \
                patch.object(rollout, 'set_dispatch', side_effect=lambda c, d, s, paused: events.append(('dispatch', paused)) or s.update(dispatchPaused=paused)), \
                patch.object(rollout, 'wait_requests_idle', side_effect=drain or (lambda c: events.append(('drain',)))), \
                patch.object(rollout, 'update_env', side_effect=lambda region, function, changes, remove=(): events.append(('env', function, changes))), \
                patch.object(rollout, 'database_status', side_effect=lambda c: status()), \
                patch.object(rollout, 'start_workers', side_effect=lambda v: events.append(('start', v.name))), \
                patch.object(rollout, 'sync_settings', side_effect=lambda *a, **k: events.append(('sync', k.get('previous')))), \
                patch.object(rollout, 'api_smoke', side_effect=lambda c, d, nodes: events.append(('api', tuple(nodes))) or d / 'api.json'), \
                patch.object(rollout, 'healthy_workers', side_effect=lambda c, d, digest: events.append(('healthy', digest))), \
                patch.object(rollout, 'run', side_effect=lambda *a: events.append(('cmd',) + a[:3])):
            rollout.rolling(config, Path(name), state)
        return state, events

    def test_rolling_prepares_a_burst_then_switches_with_dispatch_paused(self):
        state, events = self.run_rolling()
        names = [e[0] if e[0] not in ('ssm', 'prepare', 'start', 'env') else e[:2] for e in events]
        rest = (self.primary, self.other)
        self.assertEqual(state['status'], 'passed')
        self.assertEqual(state['step'], 'complete')
        self.assertEqual(state['previousDigest'], self.old)
        self.assertEqual(state['runtimeDigest'], self.new)
        self.assertTrue(rollout.SNAPSHOT_ID.fullmatch(state['aptSnapshot']))
        self.assertIn(('ssm', 'hold-bursts', (self.first, self.other)), events)
        self.assertIn(('prepare', 'first', (self.first,), None), events)
        self.assertIn(('ssm', 'switch-stop', rest), events)
        self.assertIn(('prepare', 'rest', rest, self.new), events)
        order = [('ssm', 'hold-bursts'), ('prepare', 'first'), 'dispatch', 'drain', ('ssm', 'switch-stop'),
                 ('env', 'bridge'), ('env', 'api'), ('start', 'verify-first'), 'sync', 'api', ('prepare', 'rest'),
                 ('start', 'verify-rest'), 'healthy']
        positions = [names.index(item) for item in order]
        self.assertEqual(positions, sorted(positions))
        self.assertEqual([e[1] for e in events if e[0] == 'dispatch'], [True, False])
        self.assertIn(('env', 'bridge', dict(JUDGE_RUNTIME_DIGEST=self.new, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)), events)
        self.assertIn(('env', 'api', dict(JUDGE_CPP_IMAGE=self.new)), events)
        self.assertEqual([e[1] for e in events if e[0] == 'api'], [(self.first,), (self.primary, self.first, self.other)])
        self.assertIn(('sync', self.old), events)
        tags = [e for e in events if e[0] == 'create-tags' and 'JudgeInstalledDigest' in e[-1]]
        self.assertEqual([t[2:-2] for t in tags], [(self.first,), rest])
        # Admission is never paused by a rolling update.
        self.assertFalse(any(e[0] == 'env' and e[1] == 'api' and 'JUDGE_ENABLED_RUNTIMES' in e[2] for e in events))

    def test_drain_timeout_before_switch_resumes_dispatch(self):
        def drain(config):
            raise TimeoutError('busy')
        with self.assertRaises(TimeoutError):
            self.run_rolling(drain=drain)
        # run_rolling raised before returning; check through a fresh run that records events.
        events = []
        with tempfile.TemporaryDirectory() as name, \
                patch.object(rollout, 'contest_guard'), patch.object(rollout, 'aws', return_value=dict(Timeout=0)), \
                patch.object(rollout.time, 'sleep'), \
                patch.object(rollout, 'set_dispatch', side_effect=lambda c, d, s, paused: events.append(paused) or s.update(dispatchPaused=paused)), \
                patch.object(rollout, 'wait_requests_idle', side_effect=drain), patch.object(rollout, 'ssm_step') as ssm:
            with self.assertRaises(TimeoutError):
                rollout.switch(dict(region='test', bridge='bridge'), Path(name), dict(runtimeDigest=self.new), self.first, [self.primary], Path(name))
        self.assertEqual(events, [True, False])
        ssm.assert_not_called()

    def test_resumed_switch_does_not_pause_again(self):
        state = dict(firstHost=self.first, previousDigest=self.old, aptSnapshot='20261010T000000Z', runtimeDigest=self.new,
                     packages='c' * 64, alarm_actions={'alarm': False}, digestSwitched=True, dispatchPaused=True)
        state, events = self.run_rolling(state)
        self.assertEqual([e[1] for e in events if e[0] == 'dispatch'], [False])
        self.assertFalse(any(e[0] in ('drain', 'env') for e in events))
        self.assertEqual(state['status'], 'passed')

    def test_contest_window_and_old_bridge_block_rolling(self):
        soon = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat()
        later = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=6)).isoformat().replace('+00:00', 'Z')
        base = dict(pending=0, undispatched=0, dispatchPaused=False)
        for status, allowed in [({**base, 'nextContestWindowAt': None}, True), ({**base, 'nextContestWindowAt': later}, True),
                                ({**base, 'nextContestWindowAt': soon}, False), (dict(pending=0, undispatched=0), False)]:
            with patch.object(rollout, 'database_status', return_value=status):
                if allowed:
                    rollout.contest_guard({})
                else:
                    with self.assertRaises(ValueError):
                        rollout.contest_guard({})

    def test_host_scripts_parse_and_pin_one_snapshot(self):
        script = rollout.os_update_command('a' * 32, '20261010T093000Z', 'c' * 64)
        subprocess.run(['bash', '-n'], input=script, text=True, check=True)
        self.assertIn('APT::Snapshot "%s";\\n\' 20261010T093000Z', script)
        self.assertIn('Never-Include-Phased-Updates=true', script)
        self.assertIn('exit 194', script)
        self.assertIn('c' * 64, script)
        unpinned = rollout.os_update_command('a' * 32, None)
        subprocess.run(['bash', '-n'], input=unpinned, text=True, check=True)
        self.assertNotIn('APT::Snapshot', unpinned)
        self.assertIn('rm -f /etc/apt/apt.conf.d/99judge-snapshot', unpinned)
        for bad in ('20261010', '$(reboot)'):
            with self.assertRaises(ValueError):
                rollout.os_update_command('a' * 32, bad)
        seal = rollout.seal_command(self.new)
        subprocess.run(['bash', '-n'], input=seal, text=True, check=True)
        compile(seal.split("<<'PY'\n", 1)[1].rsplit('\nPY\n', 1)[0], '<seal>', 'exec')
        with self.assertRaises(ValueError):
            rollout.seal_command('sha256:$(id)')

    def test_rest_of_pool_must_match_the_first_package_list_and_digest(self):
        scripts, commands = {}, []
        host = dict(runtimeDigest=self.new, passedRuntimes=['python314-isolate'], failedRuntimes=[])

        def ssm_step(directory, step, config, nodes, script):
            scripts[step] = script('a' * 32) if callable(script) else script
            return dict(region='test', commands={node: 'command' for node in nodes})

        def command(*args):
            commands.append(args)
            if 'submit' in args:
                directory = Path(args[args.index('--run-dir') + 1])
                directory.mkdir()
                nodes = [args[i + 1] for i, arg in enumerate(args) if arg == '--instance']
                (directory / 'report.json').write_text(json.dumps(dict(runtimeDigest=report_digest, hosts=dict.fromkeys(nodes, host))))
            return ''
        with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'ssm_step', side_effect=ssm_step), \
                patch.object(rollout, 'run', side_effect=command), \
                patch.object(rollout, 'command_output', return_value='apt output\npackages=' + 'c' * 64 + ' kernel=7.0.0-1014-aws\n'):
            directory, state = Path(name), dict(aptSnapshot='20261010T093000Z', releaseSHA256='d' * 64)
            config = dict(region='test', bucket='bucket', release='worker.tar.gz', runtimes=['python314'])
            report_digest = self.new
            rollout.prepare_hosts(config, directory, state, [self.first], 'first')
            self.assertEqual(state['packages'], 'c' * 64)
            self.assertNotIn('c' * 64, scripts['os-first'])
            rollout.prepare_hosts(config, directory, state, [self.primary, self.other], 'rest', expected_digest=self.new)
            self.assertIn('if [ "$packages" != ' + 'c' * 64, scripts['os-rest'])
            self.assertIn(self.new, scripts['seal-rest'])
            installs = [args for args in commands if 'judge/deploy-ssm.py' in args]
            self.assertEqual([a[a.index('--instance') + 1] for a in installs], [self.first, self.primary, self.other])
            self.assertTrue(all(a[a.index('--expected-sha256') + 1] == 'd' * 64 for a in installs))
            report_digest = 'sha256:' + 'e' * 64
            host['runtimeDigest'] = report_digest
            with self.assertRaises(ValueError):
                rollout.prepare_hosts(config, directory, state, [self.primary, self.other], 'again', expected_digest=self.new)

    def test_api_smoke_requires_concurrency_only_with_several_hosts(self):
        calls = []
        def command(*args):
            calls.append(args)
            Path(args[args.index('--report') + 1]).write_text('{"status":"passed"}')
            return ''
        config = dict(region='test', api='api', api_url='https://test.invalid', smoke_runtime='python314', runtimes=['python314'])
        for nodes in ([self.first], [self.first, self.primary]):
            catalog = io.BytesIO(b'{"maintenance":false,"items":[{"id":"python314"}]}')
            with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'run', side_effect=command), \
                    patch.object(rollout.urllib.request, 'urlopen', return_value=catalog):
                rollout.api_smoke(config, Path(name), nodes)
        self.assertEqual([[a[i + 1] for i, arg in enumerate(a) if arg == '--instance'] for a in calls], [[], [self.first, self.primary]])
