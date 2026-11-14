import base64
import contextlib
import datetime
import hashlib
import io
import json
import subprocess
from pathlib import Path
import sys
import tempfile
import unittest
import unittest.mock
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import rollout


class RolloutTests(unittest.TestCase):
    def test_rolling_additions_require_ready_reports_from_every_host(self):
        digest = 'sha256:' + 'a' * 64
        config = dict(region='test', bridge='bridge', api='api', nodes=['first', 'rest'],
                      runtimes=['python314'], publish_runtimes=['python314', 'whitespace13'])
        state = dict(runtimeDigest=digest, previousDigest='old')
        with tempfile.TemporaryDirectory() as tmp:
            directory = Path(tmp)
            reports = {}
            for index, node in enumerate(config['nodes']):
                run_id = str(index) * 32
                host = dict(runtimeDigest=digest, passedRuntimes=['python314-isolate', 'whitespace13-isolate'],
                            failedRuntimes=[], ready=True, runId=run_id)
                reports[node] = dict(host, hosts={node: host})
                (directory / ('verify-' + node)).mkdir()
            for fault in ('not-ready', 'missing-language', 'wrong-digest', 'missing-host', None):
                first, rest = json.loads(json.dumps([reports['first'], reports['rest']]))
                if fault == 'not-ready':
                    rest['hosts']['rest']['ready'] = False
                elif fault == 'missing-language':
                    rest['hosts']['rest']['passedRuntimes'] = ['python314-isolate']
                elif fault == 'wrong-digest':
                    rest['runtimeDigest'] = rest['hosts']['rest']['runtimeDigest'] = 'sha256:' + 'b' * 64
                elif fault == 'missing-host':
                    rest['hosts'] = {'other': rest['hosts']['rest']}
                for node, report in [('first', first), ('rest', rest)]:
                    (directory / ('verify-' + node) / 'report.json').write_text(json.dumps(report))
                with patch.object(rollout, 'update_env') as update, patch.object(rollout, 'sync_settings') as sync:
                    if fault:
                        with self.assertRaises(ValueError):
                            rollout.publish_rolling_runtimes(config, directory, state)
                        update.assert_not_called()
                        sync.assert_not_called()
                    else:
                        result = rollout.publish_rolling_runtimes(config, directory, state)
                        self.assertEqual(result['runtimes'], config['publish_runtimes'])
                        self.assertEqual([c.args[1] for c in update.call_args_list], ['bridge', 'api'])
                        self.assertTrue(all(c.args[2] == {'JUDGE_ENABLED_RUNTIMES': 'python314,whitespace13'} for c in update.call_args_list))
                        sync.assert_called_once_with(config, directory, digest, config['publish_runtimes'], previous='old')

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

    def test_lanes_follow_the_live_bridge(self):
        config = dict(region='test', bridge='bridge', queues=['qa', 'r', 'da', 'dr'], request_queue_b='qb')
        for url, expected in [('qb', dict(active='b', active_url='qb', standby_url='qa')),
                              ('qa', dict(active='a', active_url='qa', standby_url='qb'))]:
            bridge = dict(Environment=dict(Variables=dict(JUDGE_REQUEST_QUEUE_URL=url)))
            with patch.object(rollout, 'aws', return_value=bridge):
                self.assertEqual(rollout.lanes(config), expected)
        bridge = dict(Environment=dict(Variables=dict(JUDGE_REQUEST_QUEUE_URL='other')))
        with patch.object(rollout, 'aws', return_value=bridge), self.assertRaises(ValueError):
            rollout.lanes(config)

    def test_lanes_without_a_second_queue(self):
        config = dict(region='test', bridge='bridge', queues=['qa', 'r', 'da', 'dr'])
        bridge = dict(Environment=dict(Variables=dict(JUDGE_REQUEST_QUEUE_URL='qa')))
        with patch.object(rollout, 'aws', return_value=bridge):
            self.assertEqual(rollout.lanes(config), dict(active='a', active_url='qa', standby_url=None))
        self.assertEqual(rollout.all_queues(config), config['queues'])

    def test_wait_empty_watches_both_lanes(self):
        config = dict(region='test', queues=['qa', 'r', 'da', 'dr'], request_queue_b='qb')
        def attributes(region, service, operation, *args):
            busy = args[args.index('--queue-url') + 1] == 'qb'
            return dict(Attributes=dict(ApproximateNumberOfMessages='1' if busy else '0',
                                        ApproximateNumberOfMessagesNotVisible='0', ApproximateNumberOfMessagesDelayed='0'))
        with patch.object(rollout, 'database_status', return_value=dict(pending=0, undispatched=0)), \
                patch.object(rollout, 'aws', side_effect=attributes), \
                patch.object(rollout.time, 'monotonic', side_effect=[0, 1, 2, 4000]), patch.object(rollout.time, 'sleep'):
            with self.assertRaises(TimeoutError):
                rollout.wait_empty(config)

    def test_preflight_syncs_the_lane_before_terraform(self):
        config = dict(region='test', account='1', api_url='https://x', runtimes=['python314'], smoke_runtime='python314',
                      nodes=['i-0123456789abcdef0'], queues=['qa', 'r', 'da', 'dr'], request_queue_b='qb', bridge='bridge',
                      bucket='bucket', result_mapping='m', dispatch_rule='rule', worker_alarms=['alarm'],
                      repository='o/r', github_environment='dev')
        arns = {'qa': 'arn:qa', 'qb': 'arn:qb', 'r': 'arn:r', 'da': 'arn:da', 'dr': 'arn:dr'}
        def aws(region, service, operation, *args):
            if operation == 'get-caller-identity':
                return dict(Account='1')
            if operation == 'get-function-configuration':
                return dict(FunctionArn='arn:bridge', Environment=dict(Variables=dict(JUDGE_REQUEST_QUEUE_URL='qb', JUDGE_JOB_BUCKET='bucket')))
            if operation == 'get-event-source-mapping':
                return dict(FunctionArn='arn:bridge', EventSourceArn='arn:r')
            if operation == 'list-targets-by-rule':
                return dict(Targets=[dict(Arn='arn:bridge')])
            if operation == 'get-queue-attributes':
                url = args[args.index('--queue-url') + 1]
                dead = 'arn:da' if url in ('qa', 'qb') else 'arn:dr'
                return dict(Attributes=dict(QueueArn=arns[url], RedrivePolicy=json.dumps(dict(deadLetterTargetArn=dead))))
            if operation == 'describe-alarms':
                return dict(MetricAlarms=[dict(AlarmName='alarm')])
            return dict(InstanceInformationList=[dict(PingStatus='Online')])
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            for path in ('infra/api/.terraform', 'infra/judge/.terraform'):
                (root / path).mkdir(parents=True)
            tfvars = root / 'infra/judge/zz-rollout.auto.tfvars.json'
            tfvars.write_text('{"request_lane":"a","worker_count":2}')
            seen = []
            def command(*args):
                if args[0] == 'terraform':
                    seen.append(json.loads(tfvars.read_text()))
                return '[]'
            with patch.object(rollout, 'ROOT', root), patch.object(rollout, 'aws', side_effect=aws), \
                    patch.object(rollout, 'run', side_effect=command):
                rollout.preflight(config)
            self.assertEqual(seen, [dict(request_lane='b', worker_count=2)] * 2)
            # A standby lane that does not redrive to the request dead letter queue is refused.
            arns['da'], arns['dr'] = 'arn:dr', 'arn:da'
            with patch.object(rollout, 'ROOT', root), patch.object(rollout, 'aws', side_effect=aws), \
                    patch.object(rollout, 'run', side_effect=command), self.assertRaises(ValueError):
                rollout.preflight(config)

    def test_sync_lane_changes_only_the_lane_and_settings_can_carry_it(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            (root / 'infra/judge').mkdir(parents=True)
            path = root / 'infra/judge/zz-rollout.auto.tfvars.json'
            with patch.object(rollout, 'ROOT', root):
                rollout.sync_lane({}, 'b')
                self.assertEqual(json.loads(path.read_text()), dict(request_lane='b'))
                path.write_text('{"request_lane":"a","worker_count":2}')
                rollout.sync_lane({}, 'b')
                self.assertEqual(json.loads(path.read_text()), dict(request_lane='b', worker_count=2))
                (root / 'infra/api').mkdir(parents=True)
                (root / 'run').mkdir()
                values = {}
                def command(*args):
                    if args[2] == 'set':
                        values[args[3]] = args[args.index('--body') + 1]
                        return ''
                    return json.dumps([dict(name=n, value=v) for n, v in values.items()])
                config = dict(repository='o/r', github_environment='dev')
                with patch.object(rollout, 'run', side_effect=command):
                    rollout.sync_settings(config, root / 'run', 'sha256:fixed', ['python314'], lane='a')
            self.assertEqual(json.loads(path.read_text())['request_lane'], 'a')
            self.assertEqual(json.loads(path.read_text())['worker_count'], 2)
            self.assertNotIn('request_lane', json.loads((root / 'infra/api/zz-rollout.auto.tfvars.json').read_text()))

    def test_settings_sync_keeps_a_lane_written_after_the_backup(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            for path in ('infra/api', 'infra/judge', 'run'):
                (root / path).mkdir(parents=True)
            tfvars = root / 'infra/judge/zz-rollout.auto.tfvars.json'
            tfvars.write_text('{"worker_count":2}')
            values = {}
            def command(*args):
                if args[2] == 'set':
                    values[args[3]] = args[args.index('--body') + 1]
                    return ''
                return json.dumps([dict(name=n, value=v) for n, v in values.items()])
            config = dict(repository='o/r', github_environment='dev')
            with patch.object(rollout, 'ROOT', root), patch.object(rollout, 'run', side_effect=command):
                rollout.sync_settings(config, root / 'run', 'sha256:one', ['python314'], lane='a')
                rollout.sync_lane(config, 'b')
                rollout.sync_settings(config, root / 'run', 'sha256:two', ['python314'])
            self.assertEqual(json.loads(tfvars.read_text()),
                             dict(worker_count=2, request_lane='b', runtime_digest='sha256:two', enabled=True,
                                  enabled_runtimes=['python314']))

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

    def run_finish(self, count=3, **extra):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            digest = 'sha256:' + 'a' * 64
            host = dict(runtimeDigest=digest, passedRuntimes=['python314-isolate'], failedRuntimes=[], ready=True, runId='b' * 32)
            report_path = directory / 'report.json'
            nodes = ['i-0123456789abcdef' + str(i) for i in range(count)]
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

    def test_finish_passes_instances_only_with_several_hosts(self):
        for count in (1, 2):
            with self.subTest(count=count):
                state, calls, _, nodes, _ = self.run_finish(count=count)
                smoke = next(args for args in calls if 'judge/smoke-api.py' in args)
                self.assertEqual([smoke[i + 1] for i, arg in enumerate(smoke) if arg == '--instance'],
                                 nodes if count > 1 else [])
                self.assertEqual(state['apiReport'], smoke[smoke.index('--report') + 1])
                self.assertEqual((state['status'], state['step']), ('passed', 'complete'))

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

    def test_worker_health_ignores_judge_code_failures_but_not_platform_failures(self):
        digest = 'sha256:' + 'a' * 64
        with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'new_run', return_value={}), \
                patch.object(rollout, 'submit') as submit, patch.object(rollout, 'collect', return_value=0):
            rollout.healthy_workers(dict(region='test', nodes=['mi-a']), Path(name), digest)
        code = compile(submit.call_args.args[-1].split("<<'PY'\n", 1)[1].rsplit('\nPY\n', 1)[0], '<worker health>', 'exec')

        def check(*failures):
            journal = '\n'.join(json.dumps(event) for event in [dict(event='worker_started'), *failures])
            outputs = {'MainPID': '42\n', 'InvocationID': 'abc\n'}
            def check_output(args, text=False):
                return journal if args[0] == 'journalctl' else outputs[args[4]]
            host = type(sys)('host')
            host.verify_assets = lambda: digest
            with patch.dict(sys.modules, host=host), patch.object(sys, 'argv', ['-', digest]), \
                    patch('subprocess.check_output', side_effect=check_output), patch('subprocess.run'), \
                    patch.object(Path, 'read_bytes', return_value=b'JUDGE_RUNTIME_DIGEST=' + digest.encode() + b'\0'), \
                    patch('builtins.print'):
                exec(code, {'__name__': '__main__'})

        check(dict(event='failure', category='judge_code', reason='checker_execution_failed', verdict='JE'))
        for category in ('platform', 'unknown'):
            with self.assertRaisesRegex(SystemExit, 'worker reported failures'):
                check(dict(event='failure', category=category, reason='x'))


class Interrupted(Exception):
    pass


class Interrupt(list):
    """An event list that stops the run right after the first event starting with `at`, like a killed process."""

    def __init__(self, at=None):
        super().__init__()
        self.at = at

    def append(self, event):
        super().append(event)
        if self.at is not None and event[:len(self.at)] == self.at:
            self.at = None
            raise Interrupted(event)


class RollingTests(unittest.TestCase):
    first, primary, other = 'i-0123456789abcdef1', 'i-0123456789abcdef0', 'i-0123456789abcdef2'
    old, new = 'sha256:' + 'a' * 64, 'sha256:' + 'b' * 64
    window = 'revert is only possible between the lane switch and the old worker stop'

    def live_lambdas(self, url='qa'):
        """The live Lambda environments; update_env writes to them and the bridge status reads them."""
        return dict(bridge=dict(JUDGE_RUNTIME_DIGEST=self.old, JUDGE_PREVIOUS_RUNTIME_DIGEST='', JUDGE_REQUEST_QUEUE_URL=url,
                                JUDGE_ENABLED_RUNTIMES='python314'),
                    api=dict(JUDGE_CPP_IMAGE=self.old, JUDGE_ENABLED_RUNTIMES='python314'))

    def run_rolling(self, state=None, events=None, live=None, directory=None, queues=None, in_flight=(0,), requeued=0,
                    prepared=None, action=None, clock=None, purges=None, watch=None, **overrides):
        """Run rolling (or another action) against fakes; lane a is 'qa' (queues[0]) and lane b is 'qb' (request_queue_b).

        Pass the same live, queues, clock and purges to a resumed run: they are the AWS side, which outlives a process.
        """
        events = [] if events is None else events
        prepared = prepared or self.new
        config = dict(region='test', api='api', bridge='bridge', pool='judge-dev', nodes=[self.primary, self.first, self.other],
                      worker_alarms=['alarm'], runtimes=['python314'], queues=['qa', 'r', 'da', 'dr'], request_queue_b='qb')
        config = {key: value for key, value in {**config, **overrides}.items() if value is not None}
        state = {} if state is None else state
        live = self.live_lambdas() if live is None else live
        queues = {} if queues is None else queues
        counted, clock = [], [0.0] if clock is None else clock
        purges = {} if purges is None else purges

        def aws(region, service, operation, *args):
            if operation == 'get-function-configuration':
                variables = live[args[args.index('--function-name') + 1]]
                return dict(Environment=dict(Variables=dict(variables)), Timeout=120)
            if operation == 'describe-alarms':
                return dict(MetricAlarms=[dict(AlarmName='alarm', ActionsEnabled=True)])
            if operation == 'purge-queue':
                url = args[args.index('--queue-url') + 1]
                # SQS rejects a second purge of the same queue within 60 seconds.
                if url in purges and clock[0] - purges[url] < 60:
                    raise RuntimeError('PurgeQueueInProgress')
                purges[url] = clock[0]
                queues.pop(url, None)
            events.append((operation, *args))
            return {}

        def update_env(region, function, changes, remove=()):
            live[function].update(changes)
            for name in remove:
                live[function].pop(name, None)
            events.append(('env', function, changes))

        def requeue(config, digest):
            # The bridge requeues only its current previous digest.
            if digest != live['bridge']['JUDGE_PREVIOUS_RUNTIME_DIGEST']:
                raise RuntimeError('bridge did not requeue the previous digest')
            events.append(('requeue', digest))
            return requeued

        def prepare_hosts(config, directory, state, nodes, label, expected_digest=None):
            events.append(('prepare', label, tuple(nodes), expected_digest))
            verification = directory / ('verify-' + label)
            verification.mkdir(exist_ok=True)
            (verification / 'report.json').write_text(json.dumps(dict(runtimeDigest=prepared)))
            state.setdefault('packages', 'c' * 64)
            return verification

        def queue_counts(config, url):
            counted.append(url)
            events.append(('count', url))
            return queues.get(url, 0)

        def status(config):
            bridge = live['bridge']
            # in_flight[k] is what the bridge reports at the (k+1)-th check of the previous lane.
            checks = counted.count(state.get('previousQueue'))
            return dict(kind='judge-deployment-status', pending=0, undispatched=0, dispatchPaused=False, nextContestWindowAt=None,
                        runtimeDigest=bridge['JUDGE_RUNTIME_DIGEST'], previousRuntimeDigest=bridge['JUDGE_PREVIOUS_RUNTIME_DIGEST'],
                        requestQueueUrl=bridge['JUDGE_REQUEST_QUEUE_URL'],
                        previousInFlight=in_flight[min(max(checks - 1, 0), len(in_flight) - 1)])

        def sleep(seconds):
            events.append(('sleep', seconds))
            clock[0] += seconds

        fakes = dict(
            preflight=lambda c: events.append(('preflight',)), pool_bursts=lambda c: [self.first, self.other], aws=aws,
            update_env=update_env, deploy_bridge=lambda *a, **k: events.append(('bridge-code',)),
            contest_guard=lambda c: events.append(('contest',)),
            ssm_step=lambda d, step, c, nodes, script: events.append(('ssm', step, tuple(nodes))),
            prepare_hosts=prepare_hosts, database_status=status, queue_counts=queue_counts,
            set_dispatch=lambda c, d, s, paused: events.append(('dispatch', paused)), requeue=requeue,
            standby_ready=lambda c, d, node, url: events.append(('standby', node, url)),
            start_workers=lambda v, request_queue=None: events.append(('start', v.name, request_queue)),
            sync_settings=lambda c, d, digest, runtimes, **k: events.append(('sync', digest, k.get('previous'), k.get('lane'))),
            api_smoke=lambda c, d, nodes: events.append(('api', tuple(nodes))) or d / 'api.json',
            healthy_workers=lambda c, d, digest: events.append(('healthy', digest)),
            run=lambda *a: events.append(('cmd',) + a[:3]),
            # By default the watcher is silent and passes; `watch` replaces any of these.
            **{'start_watch': lambda c, d, s: object(), 'stop_watch': lambda h, d: dict(status='passed', maxWaitMs=1200),
               'collect_stale_watch': lambda c, d, s: None, **(watch or {})})
        with contextlib.ExitStack() as stack:
            name = stack.enter_context(tempfile.TemporaryDirectory())
            stack.enter_context(contextlib.redirect_stdout(io.StringIO()))
            for function, fake in fakes.items():
                stack.enter_context(patch.object(rollout, function, side_effect=fake))
            stack.enter_context(patch.object(rollout.time, 'sleep', side_effect=sleep))
            stack.enter_context(patch.object(rollout.time, 'monotonic', side_effect=lambda: clock[0]))
            (action or rollout.rolling)(config, directory or Path(name), state)
        return state, events

    def stopped_rolling(self, directory, at=None):
        """Run rolling until it stops at the event `at`, or else at the drain timeout with the old workers on lane a."""
        state, live = {}, self.live_lambdas()
        with self.assertRaises(Interrupted if at else TimeoutError):
            self.run_rolling(state, Interrupt(at), live=live, directory=directory, in_flight=(2,))
        return state, live

    def run_revert(self, state, live, directory, events=None, **kwargs):
        return self.run_rolling(state, events, live=live, directory=directory, action=rollout.revert, **kwargs)

    def reverted(self):
        """The bridge configuration a revert sets: the previous lane and digest, with the new digest as the previous one."""
        return dict(JUDGE_REQUEST_QUEUE_URL='qa', JUDGE_RUNTIME_DIGEST=self.old, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.new)

    def assertInOrder(self, events, *expected):
        position = 0
        for item in expected:
            try:
                position = events.index(item, position) + 1
            except ValueError:
                self.fail(f'{item} not found after event {position}: {events[position:]}')

    def test_rolling_switches_lanes_without_pausing_dispatch(self):
        state, events = self.run_rolling()
        rest = (self.primary, self.other)
        self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
        self.assertEqual((state['previousDigest'], state['runtimeDigest']), (self.old, self.new))
        # "active" names the lane in use after the switch (the standby before it); "previous" the lane in use before it.
        self.assertEqual((state['previousLane'], state['previousQueue'], state['activeLane'], state['activeQueue']), ('a', 'qa', 'b', 'qb'))
        self.assertEqual((state['digestSwitched'], state['apiSwitched'], state['oldWorkersStopped'], state['requeued']), (True, True, True, 0))
        switch = dict(JUDGE_REQUEST_QUEUE_URL='qb', JUDGE_RUNTIME_DIGEST=self.new, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)
        self.assertInOrder(events, ('ssm', 'hold-bursts', (self.first, self.other)), ('prepare', 'first', (self.first,), None),
                           ('count', 'qb'), ('start', 'verify-first', 'qb'), ('sleep', 35), ('standby', self.first, 'qb'),
                           ('contest',), ('env', 'bridge', switch), ('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.new)),
                           ('sync', self.new, self.old, 'b'), ('api', (self.first,)), ('count', 'qa'), ('ssm', 'switch-stop', rest),
                           ('prepare', 'rest', rest, self.new), ('start', 'verify-rest', 'qb'), ('healthy', self.new),
                           ('api', (self.primary, self.first, self.other)))
        # One bridge update switches the lane, the digest and the previous digest together; dispatch is never paused.
        self.assertEqual([e for e in events if e[:2] == ('env', 'bridge')], [('env', 'bridge', switch)])
        self.assertFalse(any(e[0] == 'dispatch' for e in events))
        # Admission is never paused by a rolling update.
        self.assertFalse(any(e[:2] == ('env', 'api') and e[2].get('JUDGE_ENABLED_RUNTIMES') == 'none' for e in events))
        self.assertFalse(any(e[0] == 'requeue' for e in events))
        tags = [e for e in events if e[0] == 'create-tags' and 'JudgeInstalledDigest' in e[-1]]
        self.assertEqual([t[2:-2] for t in tags], [(self.first,), rest])

    def watching(self, events, report=None, start=None):
        """Watcher fakes that leave their place in the event list; `report` is what the stopped watcher wrote."""
        def stop(handle, directory):
            events.append(('watch-stop',))
            return report

        def begin(config, directory, state):
            events.append(('watch-start',))
            if start:
                raise start
            return object()
        return dict(start_watch=begin, stop_watch=stop)

    def test_rolling_watches_the_switch(self):
        events = []
        with tempfile.TemporaryDirectory() as name:
            state, events = self.run_rolling(events=events, directory=Path(name),
                                             watch=self.watching(events, dict(status='passed', maxWaitMs=1200)))
            switch_at = float((Path(name) / 'switch-at').read_text())
        self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
        self.assertEqual(state['switchAt'], switch_at)
        self.assertEqual((state['switchWatch']['status'], state['switchWatch']['maxWaitMs']), ('passed', 1200))
        self.assertFalse(state.get('switchWatchFailed'))
        bridge = [e for e in events if e[:2] == ('env', 'bridge')][0]
        self.assertInOrder(events, ('standby', self.first, 'qb'), ('watch-start',), ('sleep', rollout.WATCH_LEAD), ('contest',), bridge,
                           ('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.new)), ('sleep', rollout.WATCH_TAIL),
                           ('watch-stop',), ('sync', self.new, self.old, 'b'))

    def test_a_failed_watch_finishes_every_step_but_never_reaches_complete(self):
        events, state = [], {}
        failed = dict(status='failed', maxWaitMs=30000)
        with tempfile.TemporaryDirectory() as name:
            directory, live, queues, clock = Path(name), self.live_lambdas(), {}, [0.0]
            with self.assertRaisesRegex(ValueError, r'switch latency check failed; see switch-watch\.json'):
                self.run_rolling(state, events, live=live, directory=directory, queues=queues, clock=clock,
                                 watch=self.watching(events, failed))
            self.assertTrue(state['switchWatchFailed'])
            self.assertNotEqual(state['step'], 'complete')
            self.assertNotEqual(state.get('status'), 'passed')
            self.assertInOrder(events, ('watch-stop',), ('prepare', 'rest', (self.primary, self.other), self.new),
                               ('healthy', self.new), ('api', (self.primary, self.first, self.other)),
                               ('enable-alarm-actions', '--alarm-names', 'alarm'), ('cmd', 'terraform', '-chdir=infra/api', 'apply'),
                               ('cmd', 'terraform', '-chdir=infra/judge', 'apply'))
            self.assertTrue(any(e[0] == 'create-tags' and 'JudgeMaintenanceHoldUntil' in e[-1] for e in events[events.index(('watch-stop',)):]))
            # Resuming reaches the same error without redoing a step or starting another watcher.
            again = []
            with self.assertRaisesRegex(ValueError, 'switch latency check failed'):
                self.run_rolling(state, again, live=live, directory=directory, queues=queues, clock=clock,
                                 watch=self.watching(again, failed))
            self.assertEqual([e for e in again if e[0] in ('prepare', 'start', 'api', 'env', 'watch-start', 'watch-stop')], [])

    def test_a_watcher_that_cannot_start_does_not_stop_the_switch(self):
        events, state = [], {}
        with self.assertRaisesRegex(ValueError, 'switch latency check failed'):
            self.run_rolling(state, events, watch=self.watching(events, start=OSError('no spawn')))
        self.assertTrue(state['switchWatchFailed'])
        self.assertTrue(any(e[:2] == ('env', 'bridge') for e in events))
        self.assertTrue(state['digestSwitched'] and state['apiSwitched'] and state['oldWorkersStopped'])
        self.assertFalse(('watch-stop',) in events)

    def test_a_watcher_that_died_without_a_report_fails_the_watch(self):
        events, state = [], {}
        with self.assertRaisesRegex(ValueError, 'switch latency check failed'):
            self.run_rolling(state, events, watch=self.watching(events, None))
        self.assertTrue(state['switchWatchFailed'])

    def test_resume_after_the_switch_without_a_watch_report_is_interrupted(self):
        live = self.live_lambdas('qa')
        live['bridge'].update(JUDGE_RUNTIME_DIGEST=self.new, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)
        state = dict(firstHost=self.first, previousDigest=self.old, runtimeDigest=self.new, packages='c' * 64,
                     alarm_actions={'alarm': False}, previousLane='b', previousQueue='qb', activeLane='a', activeQueue='qa',
                     digestSwitched=True, apiSwitched=False, switchWatcher=dict(pid=1))
        events = []
        state, events = self.run_rolling(state, events, live=live, watch=self.watching(events))
        self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
        self.assertEqual(state['switchWatch'], 'interrupted')
        self.assertFalse(state.get('switchWatchFailed'))
        self.assertFalse(('watch-start',) in events or ('watch-stop',) in events)

    def test_resume_uses_a_watch_report_that_finished_before_the_run_died(self):
        live = self.live_lambdas('qa')
        live['bridge'].update(JUDGE_RUNTIME_DIGEST=self.new, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)
        state = dict(firstHost=self.first, previousDigest=self.old, runtimeDigest=self.new, packages='c' * 64,
                     alarm_actions={'alarm': False}, previousLane='b', previousQueue='qb', activeLane='a', activeQueue='qa',
                     digestSwitched=True, apiSwitched=True, firstApiReport='x')
        with self.assertRaisesRegex(ValueError, 'switch latency check failed'):
            self.run_rolling(state, live=live, watch=dict(collect_stale_watch=lambda c, d, s: dict(status='failed', maxWaitMs=99999)))
        self.assertEqual(state['switchWatch']['status'], 'failed')

    def test_the_watcher_before_a_resumed_switch_is_replaced_not_duplicated(self):
        # Resumed before the switch: start_watch stops the recorded watcher first, then starts exactly one new process.
        stale = subprocess.Popen([sys.executable, '-c', 'pass'])
        stale.wait()
        state = dict(switchWatcher=dict(pid=stale.pid, stopFile='old-stop', report='old-report'))
        new = unittest.mock.Mock(pid=4242)
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            (directory / 'switch-watch.json').write_text('{"status": "failed"}')
            (directory / 'switch-watch.stop').write_text('')
            config = dict(api='api', api_url='https://api.invalid', region='test', smoke_runtime='python314')
            with patch.object(rollout.subprocess, 'Popen', return_value=new) as popen:
                self.assertIs(rollout.start_watch(config, directory, state), new)
            self.assertEqual(popen.call_count, 1)
            self.assertFalse((directory / 'switch-watch.json').exists() or (directory / 'switch-watch.stop').exists())
            self.assertEqual(state['switchWatcher']['pid'], 4242)
            args = popen.call_args.args[0]
            self.assertIn('--watch', args)
            self.assertEqual(args[args.index('--interval') + 1], '5')

    def test_rolling_refuses_a_busy_standby_lane(self):
        # A fresh run refuses before preparing; a resumed run checks again right before the standby worker starts.
        for state in ({}, dict(previousLane='a', previousQueue='qa', activeLane='b', activeQueue='qb')):
            resumed = bool(state)
            with self.subTest(resumed=resumed):
                events = []
                with self.assertRaisesRegex(ValueError, 'the standby lane is not empty'):
                    self.run_rolling(state, events, queues={'qb': 1})
                self.assertFalse(any(e[0] == 'env' for e in events))
                self.assertFalse(any(e[0] == 'start' for e in events))
                self.assertEqual(any(e[0] == 'prepare' for e in events), resumed)

    def test_rolling_requires_the_second_lane(self):
        events = []
        with self.assertRaisesRegex(ValueError, 'a rolling update needs request_queue_b'):
            self.run_rolling(events=events, request_queue_b=None)
        self.assertFalse(any(e[0] == 'env' for e in events))

    def test_drain_requeues_stranded_submissions(self):
        # The previous lane is empty but the bridge still counts two dispatched previous-digest submissions.
        state, events = self.run_rolling(in_flight=(2, 2, 2, 0), requeued=2)
        self.assertEqual(state['requeued'], 2)
        self.assertEqual(state['step'], 'complete')
        stop = events.index(('ssm', 'switch-stop', (self.primary, self.other)))
        drain = [e for e in events[:stop] if e in (('count', 'qa'), ('requeue', self.old))]
        self.assertEqual(drain, [('count', 'qa')] * 3 + [('requeue', self.old)] + [('count', 'qa')] * 3)

    def test_drain_timeout_keeps_old_workers(self):
        state, live = {}, self.live_lambdas()
        with tempfile.TemporaryDirectory() as name:
            events = []
            with self.assertRaisesRegex(TimeoutError, 'the previous lane did not drain; old workers keep running'):
                self.run_rolling(state, events, live=live, directory=Path(name), in_flight=(2,))
            self.assertFalse(any(e[:2] == ('ssm', 'switch-stop') for e in events))
            self.assertEqual([e for e in events if e[0] == 'requeue'], [('requeue', self.old)])
            self.assertEqual(state['step'], 'drain-previous')
            state, events = self.run_rolling(state, live=live, directory=Path(name))
        self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
        self.assertFalse(any(e[0] in ('env', 'standby') for e in events))
        # The resumed run goes straight from the first host's collected report to the drain.
        resumed = events.index(('prepare', 'first', (self.first,), None)) + 1
        self.assertEqual(events[resumed], ('count', 'qa'))
        self.assertIn(('ssm', 'switch-stop', (self.primary, self.other)), events)

    def test_resume_after_switch_skips_the_switch(self):
        # This run started on lane b, so the live bridge already sends to lane a; resume must use the saved lanes.
        live = self.live_lambdas('qa')
        live['bridge'].update(JUDGE_RUNTIME_DIGEST=self.new, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)
        state = dict(firstHost=self.first, previousDigest=self.old, runtimeDigest=self.new, packages='c' * 64,
                     alarm_actions={'alarm': False}, previousLane='b', previousQueue='qb', activeLane='a', activeQueue='qa',
                     digestSwitched=True, apiSwitched=False)
        state, events = self.run_rolling(state, live=live)
        self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
        self.assertFalse(any(e[:2] == ('env', 'bridge') or e[0] == 'standby' for e in events))
        self.assertFalse(any(e[:2] == ('start', 'verify-first') for e in events))
        resumed = events.index(('prepare', 'first', (self.first,), None)) + 1
        self.assertEqual(events[resumed:resumed + 2], [('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.new))])
        self.assertInOrder(events[resumed:], ('sync', self.new, self.old, 'a'), ('api', (self.first,)), ('count', 'qb'),
                           ('ssm', 'switch-stop', (self.primary, self.other)), ('start', 'verify-rest', 'qa'))

    def test_resume_after_an_unrecorded_switch_does_not_switch_again(self):
        # The bridge took the update but the run stopped before saving digestSwitched; the new lane now has work.
        live = self.live_lambdas()
        live['bridge'].update(JUDGE_REQUEST_QUEUE_URL='qb', JUDGE_RUNTIME_DIGEST=self.new, JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)
        state = dict(firstHost=self.first, previousDigest=self.old, runtimeDigest=self.new, packages='c' * 64,
                     alarm_actions={'alarm': False}, previousLane='a', previousQueue='qa', activeLane='b', activeQueue='qb')
        state, events = self.run_rolling(state, live=live, queues={'qb': 1})
        self.assertEqual((state['status'], state['step']), ('passed', 'complete'))
        self.assertTrue(state['digestSwitched'])
        self.assertFalse(any(e[:2] == ('env', 'bridge') or e[0] == 'standby' or e == ('count', 'qb') for e in events))
        self.assertIn(('env', 'api', dict(JUDGE_CPP_IMAGE=self.new)), events)

    def test_rolling_refuses_an_unchanged_digest(self):
        # No package or release change reproduces the published digest; the bridge would get previous == current.
        events = []
        with self.assertRaisesRegex(ValueError, 'nothing changed'):
            self.run_rolling(events=events, prepared=self.old)
        self.assertIn(('prepare', 'first', (self.first,), None), events)
        self.assertFalse(any(e[0] in ('env', 'start', 'standby') for e in events))

    def test_unchanged_digest_does_not_block_a_switched_resume(self):
        # A run that already switched (recorded, or found on the live bridge) carries on; only a new switch is refused.
        lanes = dict(previousLane='a', previousQueue='qa', activeLane='b', activeQueue='qb')
        base = dict(firstHost=self.first, previousDigest=self.old, runtimeDigest=self.old, packages='c' * 64,
                    alarm_actions={'alarm': False}, **lanes)
        for recorded in (True, False):
            with self.subTest(recorded=recorded):
                live = self.live_lambdas()
                live['bridge'].update(JUDGE_REQUEST_QUEUE_URL='qb', JUDGE_PREVIOUS_RUNTIME_DIGEST=self.old)
                state = dict(base, digestSwitched=True) if recorded else dict(base)
                state, events = self.run_rolling(state, live=live, prepared=self.old)
                self.assertEqual((state['status'], state['step'], state['digestSwitched']), ('passed', 'complete', True))
                self.assertFalse(any(e[:2] == ('env', 'bridge') or e[0] == 'standby' for e in events))

    def test_revert_restores_the_previous_lane_and_digest(self):
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            # The drain timed out: users are judged on lane b, the old workers still run on lane a.
            state, live = self.stopped_rolling(directory)
            self.assertTrue(state['digestSwitched'] and state['apiSwitched'])
            # The new lane still holds work the new worker had not taken.
            state, events = self.run_revert(state, live, directory, queues={'qb': 2}, requeued=3)
        tag = ('delete-tags', '--resources', self.first, '--tags', 'Key=JudgeInstalledDigest')
        self.assertEqual(events, [('env', 'bridge', self.reverted()), ('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.old)),
                                  ('ssm', 'revert-stop', (self.first,)), tag, ('purge-queue', '--queue-url', 'qb'), ('count', 'qb'),
                                  ('requeue', self.new), ('sync', self.old, self.new, 'a'),
                                  ('enable-alarm-actions', '--alarm-names', 'alarm')])
        self.assertEqual((state['status'], state['step']), ('reverted', 'reverted'))
        self.assertEqual((state['requeued'], state['revertRequeued']), (3, 3))
        self.assertEqual({key: live['bridge'][key] for key in self.reverted()}, self.reverted())
        self.assertEqual(live['api']['JUDGE_CPP_IMAGE'], self.old)

    def test_revert_refused_outside_its_window(self):
        config = dict(region='test', api='api', bridge='bridge', runtimes=['python314'], worker_alarms=['alarm'])
        saved = dict(previousLane='a', previousQueue='qa', activeLane='b', activeQueue='qb', previousDigest=self.old,
                     runtimeDigest=self.new, firstHost=self.first, alarm_actions={'alarm': True})
        with tempfile.TemporaryDirectory() as name:
            before, _ = self.stopped_rolling(Path(name), at=('standby',))
        cases = [('a fresh run', {}, False),
                 ('stopped before the switch', before, False),
                 ('refused right before the bridge update', dict(saved, step='switch-lane'), False),
                 ('old workers stopped', dict(digestSwitched=True, oldWorkersStopped=True), False),
                 # switch-stop exists from the moment the old workers are told to stop, before oldWorkersStopped is saved.
                 ('old workers told to stop', dict(saved, digestSwitched=True, apiSwitched=True), True)]
        for case, state, stopping in cases:
            with self.subTest(case), tempfile.TemporaryDirectory() as name, patch.object(subprocess, 'run') as process, \
                    patch.object(rollout.time, 'sleep') as sleep:
                directory, unchanged = Path(name), json.dumps(state, sort_keys=True)
                if stopping:
                    (directory / 'switch-stop').mkdir()
                with self.assertRaisesRegex(ValueError, self.window):
                    rollout.revert(config, directory, state)
                # Not one AWS, gh or Terraform call, and no state change.
                process.assert_not_called()
                sleep.assert_not_called()
                self.assertEqual(json.dumps(state, sort_keys=True), unchanged)
                self.assertFalse((directory / 'state.json').exists())

    def test_rolling_refuses_a_reverted_run(self):
        for state, message in [(dict(status='reverted'), 'this run was reverted; start a new run'),
                               (dict(status='failed', step='revert-purge', digestSwitched=True, revertStarted=True),
                                'this run is being reverted; finish it with rolling-revert')]:
            with self.subTest(state['status']):
                events = []
                with self.assertRaisesRegex(ValueError, message):
                    self.run_rolling(state, events)
                self.assertEqual(events, [])

    def test_revert_after_an_unrecorded_switch(self):
        # The bridge took the update, but the run stopped before saving digestSwitched or switching the API.
        with tempfile.TemporaryDirectory() as name:
            directory = Path(name)
            state, live = self.stopped_rolling(directory, at=('env', 'bridge'))
            self.assertNotIn('digestSwitched', state)
            self.assertEqual(live['bridge']['JUDGE_REQUEST_QUEUE_URL'], 'qb')
            state, events = self.run_revert(state, live, directory)
        self.assertEqual((state['status'], state['digestSwitched']), ('reverted', True))
        self.assertEqual([e for e in events if e[0] == 'env'],
                         [('env', 'bridge', self.reverted()), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.old))])
        self.assertInOrder(events, ('env', 'bridge', self.reverted()), ('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.old)),
                           ('ssm', 'revert-stop', (self.first,)), ('purge-queue', '--queue-url', 'qb'), ('requeue', self.new),
                           ('sync', self.old, self.new, 'a'))
        self.assertEqual({key: live['bridge'][key] for key in self.reverted()}, self.reverted())

    def test_revert_refuses_a_switch_the_bridge_did_not_take(self):
        # The run sent the bridge update, which did not take effect; the bridge still sends the old digest to lane a.
        state = dict(previousLane='a', previousQueue='qa', activeLane='b', activeQueue='qb', previousDigest=self.old,
                     runtimeDigest=self.new, firstHost=self.first, alarm_actions={'alarm': True}, laneSwitchAttempted=True,
                     step='switch-lane', status='failed')
        unchanged, events = json.dumps(state, sort_keys=True), []
        with tempfile.TemporaryDirectory() as name:
            with self.assertRaisesRegex(ValueError, self.window):
                self.run_revert(state, self.live_lambdas(), Path(name), events)
            self.assertFalse((Path(name) / 'state.json').exists())
        self.assertEqual(events, [])
        self.assertEqual(json.dumps(state, sort_keys=True), unchanged)

    def test_revert_sets_the_api_back_whether_or_not_it_was_switched(self):
        tag = ('delete-tags', '--resources', self.first, '--tags', 'Key=JudgeInstalledDigest')
        # Stopped in the bridge wait (API not switched), after the API update but before saving it, and after the switch.
        for at in (('sleep', 125), ('env', 'api'), None):
            with self.subTest(at=at), tempfile.TemporaryDirectory() as name:
                directory = Path(name)
                state, live = self.stopped_rolling(directory, at=at)
                self.assertEqual(live['api']['JUDGE_CPP_IMAGE'], self.old if at == ('sleep', 125) else self.new)
                state, events = self.run_revert(state, live, directory)
                self.assertEqual(state['status'], 'reverted')
                self.assertEqual(live['api']['JUDGE_CPP_IMAGE'], self.old)
                self.assertEqual([e for e in events if e[:2] == ('env', 'api')], [('env', 'api', dict(JUDGE_CPP_IMAGE=self.old))])
                self.assertInOrder(events, ('env', 'bridge', self.reverted()), ('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.old)),
                                   ('ssm', 'revert-stop', (self.first,)), tag)

    def test_interrupted_revert_resumes_without_repeating_its_changes(self):
        points = [('env', 'bridge'), ('sleep', 125), ('env', 'api'), ('ssm', 'revert-stop'), ('delete-tags',), ('purge-queue',),
                  ('count', 'qb'), ('requeue',), ('sync',), ('enable-alarm-actions',)]
        for at in points:
            with self.subTest(at=at), tempfile.TemporaryDirectory() as name:
                directory = Path(name)
                state, live = self.stopped_rolling(directory)
                world = dict(queues={'qb': 2}, clock=[0.0], purges={}, requeued=3)
                first = Interrupt(at)
                with self.assertRaises(Interrupted):
                    self.run_revert(state, live, directory, first, **world)
                self.assertTrue(state['revertStarted'])
                state, second = self.run_revert(state, live, directory, **world)
                events = first + second
                self.assertEqual((state['status'], state['step']), ('reverted', 'reverted'))
                # The bridge update and the purge happen once; a requeue whose answer was lost is sent again (a no-op then).
                self.assertEqual([e for e in events if e[:2] == ('env', 'bridge')], [('env', 'bridge', self.reverted())])
                self.assertEqual(sum(e[0] == 'purge-queue' for e in events), 1)
                self.assertEqual(sum(e[0] == 'requeue' for e in events), 2 if at == ('requeue',) else 1)
                self.assertEqual((state['requeued'], state['revertRequeued']), (3, 3))
                self.assertInOrder(events, ('env', 'bridge', self.reverted()), ('sleep', 125), ('env', 'api', dict(JUDGE_CPP_IMAGE=self.old)),
                                   ('ssm', 'revert-stop', (self.first,)), ('purge-queue', '--queue-url', 'qb'), ('requeue', self.new),
                                   ('sync', self.old, self.new, 'a'), ('enable-alarm-actions', '--alarm-names', 'alarm'))
                # Whichever run sets the API back has waited out the bridge timeout itself.
                for run in (first, second):
                    for index, event in enumerate(run):
                        if event[:2] == ('env', 'api'):
                            self.assertIn(('sleep', 125), run[:index])
                self.assertEqual({key: live['bridge'][key] for key in self.reverted()}, self.reverted())
                self.assertEqual(live['api']['JUDGE_CPP_IMAGE'], self.old)

    def test_resumed_revert_purges_again_only_after_the_sqs_interval(self):
        # SQS deletes a purge's messages within 60 seconds and rejects another purge of the queue within 60 seconds.
        for left in (0, 1):
            with self.subTest(left=left), tempfile.TemporaryDirectory() as name:
                directory = Path(name)
                state, live = self.stopped_rolling(directory)
                world = dict(queues={'qb': 2}, clock=[0.0], purges={})
                with self.assertRaises(Interrupted):
                    self.run_revert(state, live, directory, Interrupt(('purge-queue',)), **world)
                purged = world['purges']['qb']
                world['queues']['qb'] = left  # what the lane still holds when the revert resumes at once
                state, events = self.run_revert(state, live, directory, **world)
                self.assertEqual(events[:2], [('sleep', rollout.PURGE_INTERVAL), ('count', 'qb')])
                self.assertGreaterEqual(rollout.PURGE_INTERVAL, 60)
                self.assertEqual(sum(e[0] == 'purge-queue' for e in events), left)
                if left:
                    self.assertGreaterEqual(world['purges']['qb'] - purged, 60)
                self.assertEqual(state['status'], 'reverted')
                self.assertFalse(any(e[0] == 'env' for e in events))

    def test_main_reverts_without_rollout_checks_and_keeps_reverted_runs_closed(self):
        with tempfile.TemporaryDirectory() as name:
            root = Path(name)
            # The artifacts are gone: a revert uses neither, and a refused run never reads them.
            config = dict(account='123', region='test', api='api', release=str(root / 'gone'), bridge_package=str(root / 'gone'))
            config_path = root / 'config.json'
            config_path.write_text(json.dumps(config))
            directory = root / 'run'
            directory.mkdir()
            state_path = directory / 'state.json'

            def main(action, revert=None):
                argv = ['rollout.py', action, '--config', str(config_path), '--run-dir', str(directory)]
                with contextlib.ExitStack() as stack:
                    for patcher in (patch.object(sys, 'argv', argv), patch.object(rollout, 'ROOT', root), patch.object(rollout.os, 'chdir'),
                                    patch.object(rollout, 'LOG_DIRECTORY', None)):
                        stack.enter_context(patcher)
                    aws = stack.enter_context(patch.object(rollout, 'aws', return_value=dict(Account='123')))
                    pool = stack.enter_context(patch.object(rollout, 'ensure_pool'))
                    output = stack.enter_context(contextlib.redirect_stdout(io.StringIO()))
                    if revert:
                        stack.enter_context(patch.object(rollout, 'revert', side_effect=revert))
                    try:
                        rollout.main()
                    finally:
                        # Only the account check reaches AWS, and no host is started or held.
                        self.assertEqual(aws.call_count, 1)
                        pool.assert_not_called()
                return output.getvalue()

            with self.assertRaisesRegex(ValueError, self.window):
                main('rolling-revert')
            self.assertFalse(state_path.exists())

            def stopped(config, directory, state):
                state['revertStarted'] = True
                raise RuntimeError('purge failed')
            with self.assertRaisesRegex(RuntimeError, 'purge failed'):
                main('rolling-revert', stopped)
            saved = json.loads(state_path.read_text())
            self.assertEqual((saved['status'], saved['error'], saved['revertStarted']), ('failed', 'purge failed', True))
            with self.assertRaisesRegex(ValueError, 'this run is being reverted; finish it with rolling-revert'):
                main('rolling')
            self.assertEqual(json.loads(state_path.read_text()), saved)

            state_path.write_text(json.dumps(dict(saved, status='reverted', step='reverted')))
            for action in ('rolling', 'run'):
                with self.subTest(action), self.assertRaisesRegex(ValueError, 'this run was reverted; start a new run'):
                    main(action)
            self.assertIn('already reverted', main('rolling-revert'))
            self.assertEqual(json.loads(state_path.read_text())['status'], 'reverted')

    def test_requeue_returns_the_count_and_rejects_function_errors(self):
        payloads = []
        def invoke(response, error=None):
            def aws(region, *args):
                payloads.append(json.loads(args[args.index('--payload') + 1]))
                Path(args[-1]).write_text(json.dumps(response))
                return dict(FunctionError=error) if error else {}
            return aws
        config = dict(region='test', bridge='bridge')
        with patch.object(rollout, 'aws', side_effect=invoke(dict(kind='judge-requeue', requeued=3))):
            self.assertEqual(rollout.requeue(config, self.old), 3)
        self.assertEqual(payloads, [dict(operation='requeue-digest', digest=self.old)])
        for response, error in [(dict(errorMessage='invalid operation'), 'Unhandled'), (dict(kind='other', requeued=1), None)]:
            with patch.object(rollout, 'aws', side_effect=invoke(response, error)), self.assertRaises(RuntimeError):
                rollout.requeue(config, self.old)

    def test_start_workers_passes_the_lane_only_when_given(self):
        with tempfile.TemporaryDirectory() as name:
            (Path(name) / 'receipt.json').write_text('{"phase":"smoke"}')
            calls = []
            with patch.object(rollout, 'run', side_effect=lambda *a: calls.append(a[1:])):
                rollout.start_workers(Path(name), request_queue='qb')
                rollout.start_workers(Path(name))
        self.assertEqual([c for c in calls if c[1] == 'start'],
                         [('judge/verify.py', 'start', '--run-dir', name, '--request-queue', 'qb'),
                          ('judge/verify.py', 'start', '--run-dir', name)])

    def test_standby_check_requires_the_lane_and_no_receive_failure(self):
        url = 'https://sqs.ap-northeast-1.amazonaws.com/123456789012/judge-dev-judge-requests-b'
        with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'new_run', return_value={}), \
                patch.object(rollout, 'submit') as submit, patch.object(rollout, 'collect', return_value=0):
            rollout.standby_ready(dict(region='test'), Path(name), self.first, url)
        self.assertEqual(submit.call_args.args[2], self.first)
        script = submit.call_args.args[-1]
        subprocess.run(['bash', '-n'], input=script, text=True, check=True)
        self.assertIn(url + ' ' + str(rollout.STANDBY_WINDOW) + " <<'PY'", script)
        self.assertGreater(rollout.STANDBY_WINDOW, rollout.STANDBY_WAIT)
        code = compile(script.split("<<'PY'\n", 1)[1].rsplit('\nPY\n', 1)[0], '<standby check>', 'exec')
        now = 1_800_000_000.0

        def failure(age):
            # The record shape telemetry.emit writes: an aware UTC ISO timestamp.
            stamp = datetime.datetime.fromtimestamp(now - age, datetime.timezone.utc).isoformat()
            return dict(timestamp=stamp, event='failure', category='platform', reason='queue_receive_failed')

        def check(queue, *failures):
            started = dict(timestamp=datetime.datetime.fromtimestamp(now - 600, datetime.timezone.utc).isoformat(), event='worker_started')
            journal = '\n'.join(json.dumps(event) for event in [started, *failures])
            outputs = {'MainPID': '42\n', 'InvocationID': 'abc\n'}
            def check_output(args, text=False):
                return journal if args[0] == 'journalctl' else outputs[args[4]]
            with patch.object(sys, 'argv', ['-', url, str(rollout.STANDBY_WINDOW)]), patch('time.time', return_value=now), \
                    patch('subprocess.check_output', side_effect=check_output), patch('subprocess.run'), patch('builtins.print'), \
                    patch.object(Path, 'read_bytes', return_value=b'JUDGE_REQUEST_QUEUE_URL=' + queue.encode() + b'\0'):
                exec(code, {'__name__': '__main__'})

        check(url)
        with self.assertRaisesRegex(SystemExit, 'another request queue'):
            check(url.removesuffix('-b'))
        # Failures from before a fix stay in the same invocation's journal; they must not block the resumed run.
        check(url, failure(300), failure(295), failure(rollout.STANDBY_WINDOW + 1))
        for recent in (failure(rollout.STANDBY_WAIT), failure(5),
                       dict(event='failure', category='platform', reason='queue_receive_failed')):
            with self.subTest(recent=recent), self.assertRaisesRegex(SystemExit, 'cannot receive'):
                check(url, failure(300), recent)
        with tempfile.TemporaryDirectory() as name, patch.object(rollout, 'new_run', return_value={}), \
                patch.object(rollout, 'submit'), patch.object(rollout, 'collect', return_value=1), self.assertRaises(RuntimeError):
            rollout.standby_ready(dict(region='test'), Path(name), self.first, url)

    def test_contest_guard_requires_the_lane_fields(self):
        base = dict(pending=0, undispatched=0, dispatchPaused=False, nextContestWindowAt=None)
        for missing in ('requestQueueUrl', 'previousInFlight'):
            status = dict(base, requestQueueUrl='queue', previousInFlight=0)
            del status[missing]
            with patch.object(rollout, 'database_status', return_value=status), self.assertRaises(ValueError):
                rollout.contest_guard({})

    def test_contest_window_and_old_bridge_block_rolling(self):
        soon = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=1)).isoformat()
        later = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(hours=6)).isoformat().replace('+00:00', 'Z')
        base = dict(pending=0, undispatched=0, dispatchPaused=False, requestQueueUrl='queue', previousInFlight=0)
        for status, allowed in [({**base, 'nextContestWindowAt': None}, True), ({**base, 'nextContestWindowAt': later}, True),
                                ({**base, 'nextContestWindowAt': soon}, False), (dict(pending=0, undispatched=0), False),
                                (dict(pending=0, undispatched=0, dispatchPaused=False, nextContestWindowAt=None), False)]:
            with patch.object(rollout, 'database_status', return_value=status):
                if allowed:
                    rollout.contest_guard({})
                else:
                    with self.assertRaises(ValueError):
                        rollout.contest_guard({})

    def test_host_scripts_parse_and_fail_on_stale_indexes(self):
        script = rollout.os_update_command('a' * 32, 'c' * 64)
        subprocess.run(['bash', '-n'], input=script, text=True, check=True)
        # A failed index fetch is only a warning by default and would upgrade nothing.
        self.assertIn('apt-get update --error-on=any &&', script)
        self.assertIn('Never-Include-Phased-Updates=true', script)
        self.assertIn('rm -f /etc/apt/apt.conf.d/99judge-snapshot', script)
        self.assertNotIn('APT::Snapshot', script)
        self.assertIn('exit 194', script)
        self.assertIn('c' * 64, script)
        first = rollout.os_update_command('a' * 32)
        subprocess.run(['bash', '-n'], input=first, text=True, check=True)
        self.assertNotIn('packages differ', first)
        with self.assertRaises(ValueError):
            rollout.os_update_command('a' * 32, '$(reboot)')
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
            directory, state = Path(name), dict(releaseSHA256='d' * 64)
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
