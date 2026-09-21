import base64
import sys
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import sandbox
import host
import worker
from runtimes import RUNTIMES


class RunnerTests(unittest.TestCase):
    def test_memory_limits_reach_isolate_and_reject_invalid_jobs(self):
        import tempfile
        from subprocess import CompletedProcess
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   timeLimitMs=1000, cases=[dict(input='', output='')])
        for memory in (None, True, '315', 315.5, 512.0, 63, 513):
            with self.subTest(invalid=memory), patch.object(sandbox, 'invoke') as invoke:
                job['memoryLimitMb'] = memory
                with self.assertRaisesRegex(ValueError, 'memory limit'):
                    host.validate_job(job, 'sha256:test')
                with self.assertRaisesRegex(ValueError, 'invalid limits'):
                    sandbox.execute(job)
                invoke.assert_not_called()
        for memory in (64, 315, 512):
            with self.subTest(memory=memory), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / 'box').mkdir()
                artifact, meta = root / 'main', root / 'meta'
                artifact.write_bytes(b'program')
                limits = []
                def invoke(args, **kwargs):
                    if '--run' in args:
                        limits.append(next(arg for arg in args if arg.startswith('--cg-mem=')))
                        meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                        (root / 'box/stdout').write_bytes(b'')
                        (root / 'box/stderr').write_bytes(b'')
                    return CompletedProcess(args, 0, stdout=str(root).encode())
                job['memoryLimitMb'] = memory
                with patch.object(sandbox, 'invoke', invoke), patch.object(sandbox, 'META', meta), \
                        patch.object(sandbox, 'ARTIFACT', artifact), \
                        patch.object(sandbox, 'collect_artifact', return_value=b'compiled'):
                    host.validate_job(job, 'sha256:test')
                    sandbox.execute(job, compile_phase=True)
                    sandbox.execute(job)
                self.assertEqual(limits, ['--cg-mem=1048576', f'--cg-mem={memory * 1024}'])

    def test_two_tles_skip_remaining_cases_only_for_normal_submissions(self):
        import tempfile
        for mode in (None, 'easyTest', 'validate', 'generate', 'interactive'):
            for verdicts in (['TLE', 'AC', 'TLE', 'AC'], ['TLE', 'TLE', 'AC', 'AC'],
                             ['RE', 'TLE', 'TLE', 'AC'], ['TLE', 'AC', 'AC', 'AC']):
                with self.subTest(mode=mode, verdicts=verdicts):
                    job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                               memoryLimitMb=512, timeLimitMs=1000,
                               cases=[dict(input=str(i), output='') for i in range(4)])
                    if mode == 'interactive':
                        job['interactor'] = dict(runtime='cpp17', source='int main(){}')
                    elif mode:
                        job[mode] = True
                    if mode == 'generate':
                        job['generationPrefix'] = 'test-files/' + 'a' * 32 + '/11111111-1111-4111-8111-111111111111/generated/'
                    executed, progress = [], []
                    def reply(index):
                        executed.append(index)
                        return dict(verdict=verdicts[index], cpuTimeMs=1, wallTimeMs=1, memoryBytes=1024)
                    def execute(request, compile_phase=False, **kwargs):
                        if compile_phase:
                            return dict(compiled=True)
                        item = reply(int(base64.b64decode(request['input'])))
                        return dict(status='TO' if item['verdict'] == 'TLE' else '', oom=False,
                                    overflow=False, exitCode=1 if item['verdict'] == 'RE' else 0,
                                    signal=0, output='', **item)
                    def interact(job, case, artifact, diagnostic):
                        return reply(int(case['input']))
                    with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                            patch.object(host.interactive, 'execute', interact), \
                            patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                            patch.object(sandbox, 'META', Path(tmp) / 'meta'):
                        result = host.judge(job, 'sha256:test', progress=lambda *args: progress.append(args))
                    stop = 4
                    if mode in (None, 'interactive') and verdicts.count('TLE') >= 2:
                        stop = [i for i, v in enumerate(verdicts) if v == 'TLE'][1] + 1
                    self.assertEqual(executed, list(range(stop)))
                    self.assertEqual(result['total'], 4)
                    self.assertEqual(result['passed'], verdicts[:stop].count('AC'))
                    self.assertEqual(progress[-1][1:3], (stop, 4))
                    self.assertEqual(result['verdict'], verdicts[0])
                    self.assertEqual(result['cases'][stop:], [dict(name=f'ケース{i + 1}', verdict='SKIPPED')
                                                           for i in range(stop, 4)])

    def test_compiler_failure_preserves_roslyn_stdout_diagnostics(self):
        import tempfile
        from subprocess import CompletedProcess
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'box').mkdir()
            meta = root / 'meta'
            def invoke(args, **kwargs):
                if '--run' in args:
                    meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024\nstatus:RE\nexitcode:1')
                    (root / 'box/stdout').write_bytes(b'main.cs: error CS1002: ; expected')
                    (root / 'box/stderr').write_bytes(b'')
                    return CompletedProcess(args, 1, stdout=b'')
                return CompletedProcess(args, 0, stdout=str(root).encode())
            with patch.object(sandbox, 'invoke', invoke), patch.object(sandbox, 'META', meta):
                result = sandbox.execute({'runtime': 'csharp14-isolate', 'source': 'invalid C#'}, True)
            self.assertFalse(result['compiled'])
            self.assertIn('CS1002', result['compileLog'])

    def test_progress_is_rate_limited_and_failure_does_not_fail_judging(self):
        import json
        client = Mock()
        report = worker.progress_reporter(client, 'queue', dict(submissionId='id', attemptId='attempt'))
        with patch.object(worker.time, 'monotonic', side_effect=[0, .1, .2, .3, 1.2, 2.3, 3.4]):
            report('PREPARING', 0, 4)
            report('JUDGING', 0, 4)
            report('JUDGING', 1, 4)
            report('JUDGING', 2, 4)
            report('JUDGING', 3, 4)
            self.assertEqual(client.send_message.call_count, 3)
            payload = json.loads(client.send_message.call_args.kwargs['MessageBody'])
            self.assertEqual(payload['progress'], dict(phase='JUDGING', completed=3, total=4))
            client.send_message.side_effect = RuntimeError('network down')
            report('JUDGING', 4, 4)
            report('JUDGING', 4, 4)
            self.assertEqual(client.send_message.call_count, 4)

    def test_first_failure_bypasses_progress_throttle(self):
        import json
        for verdict in ('WA', 'TLE', 'MLE', 'OLE', 'RE'):
            client = Mock()
            report = worker.progress_reporter(client, 'queue', dict(submissionId='id', attemptId='attempt'))
            with patch.object(worker.time, 'monotonic', side_effect=[0, .1, .2, 1.2]):
                report('JUDGING', 0, 4)
                report('JUDGING', 1, 4, verdict)
                self.assertEqual(client.send_message.call_count, 2)
                self.assertEqual(json.loads(client.send_message.call_args.kwargs['MessageBody'])['progress'],
                                 dict(phase='JUDGING', completed=1, total=4, verdict=verdict))
                report('JUDGING', 2, 4, verdict)
                self.assertEqual(client.send_message.call_count, 2)
                report('JUDGING', 3, 4, verdict)
                self.assertEqual(json.loads(client.send_message.call_args.kwargs['MessageBody'])['progress']['verdict'], verdict)

    def test_all_runtime_commands_are_operator_owned_and_have_smoke_fixtures(self):
        import json
        fixtures = json.loads((Path(__file__).resolve().parents[1] / 'language-smoke.json').read_text())
        self.assertEqual(set(fixtures), set(RUNTIMES))
        for name, runtime in RUNTIMES.items():
            self.assertTrue(runtime['compile'][0].startswith(('/opt/judge-runtimes/', '/usr/bin/')))
            self.assertTrue(runtime['run'][0].startswith(('/opt/judge-runtimes/', '/box/')))
            self.assertTrue({'AC', 'WA', 'CE', 'TLE'} <= {case['verdict'] for case in fixtures[name]})

    def test_java_artifact_rejects_symlink_directory(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp)
            (box / 'classes').mkdir()
            (box / 'classes' / 'escape').symlink_to('/etc', target_is_directory=True)
            with self.assertRaisesRegex(ValueError, 'class directory'):
                sandbox.collect_artifact(box, RUNTIMES['java24-isolate'])

    def test_java_artifact_preserves_nested_classes(self):
        import tempfile
        import zipfile
        import io
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp)
            (box / 'classes' / 'nested').mkdir(parents=True)
            (box / 'classes' / 'Main.class').write_bytes(b'main')
            (box / 'classes' / 'nested' / 'Helper.class').write_bytes(b'helper')
            with zipfile.ZipFile(io.BytesIO(sandbox.collect_artifact(box, RUNTIMES['java24-isolate']))) as jar:
                self.assertEqual(set(jar.namelist()), {'Main.class', 'nested/Helper.class'})

    def test_missing_and_nonfinite_measurements_fail_closed(self):
        for text in ['time:0\ntime-wall:0', 'time:nan\ntime-wall:0\ncg-mem:1',
                     'status:XX\ntime:0\ntime-wall:0\ncg-mem:1']:
            with self.assertRaises((KeyError, ValueError, RuntimeError)):
                sandbox.metadata(text)
        self.assertEqual(sandbox.metadata('time:0.125\ntime-wall:0.3\ncg-mem:2048')['memoryBytes'], 2097152)

    def test_signal_137_is_not_automatically_mle(self):
        reply = dict(index=0, status='SG', oom=False, overflow=False, exitCode=0, signal=9,
                     cpuTimeMs=1, wallTimeMs=1, memoryBytes=4096, output='')
        case = {'output': ''}
        self.assertEqual(host.case_result(reply, 0, case)['verdict'], 'RE')
        reply['oom'] = True
        self.assertEqual(host.case_result(reply, 0, case)['verdict'], 'MLE')
        reply.update(oom=False, status='TO')
        self.assertEqual(host.case_result(reply, 0, case)['verdict'], 'TLE')
        reply['overflow'] = True
        self.assertEqual(host.case_result(reply, 0, case)['verdict'], 'OLE')
        reply['memoryBytes'] = float('nan')
        with self.assertRaises(ValueError): host.case_result(reply, 0, case)

    def test_judge_keeps_expected_output_private_and_compiles_once(self):
        calls = []
        progress = []
        def execute(request, compile_phase=False):
            calls.append((request, compile_phase))
            if compile_phase:
                return {'compiled': True, 'compileLog': ''}
            return dict(status='', oom=False, overflow=False, exitCode=0, signal=0,
                        cpuTimeMs=0, wallTimeMs=1, memoryBytes=1024,
                        output=base64.b64encode(b'3\n').decode())
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   memoryLimitMb=315, timeLimitMs=1000,
                   cases=[dict(name='a', input='1', output='secret'), dict(name='b', input='2', output='3')])
        import tempfile
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', progress=lambda *args: progress.append(args))
        self.assertEqual(result['verdict'], 'WA')
        self.assertEqual(result['passed'], 1)
        self.assertEqual([compile_phase for _, compile_phase in calls], [True, False, False])
        self.assertEqual([request['memoryLimitMb'] for request, phase in calls if not phase], [315, 315])
        self.assertNotIn('secret', str(calls))
        self.assertEqual(result['cases'][0]['cpuTimeMs'], 0)
        self.assertTrue(all('sampleDetails' not in case for case in result['cases']))
        self.assertEqual(progress, [('PREPARING', 0, 2), ('JUDGING', 0, 2), ('JUDGING', 1, 2, 'WA'), ('JUDGING', 2, 2, 'WA')])

    def test_sample_details_include_file_inputs_and_actual_output_on_failure(self):
        import tempfile
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   easyTest=True, memoryLimitMb=512, timeLimitMs=1000,
                   cases=[dict(name='sample_1', input='', output='', inputFile=dict(
                       id='33333333-3333-4333-8333-333333333333', size=4, sha256='a' * 64,
                       key='test-files/' + 'a' * 32 + '/33333333-3333-4333-8333-333333333333/33333333-3333-4333-8333-333333333333',
                       versionId='v'))])
        def execute(request, compile_phase=False):
            if compile_phase:
                return dict(compiled=True)
            return dict(status='RE', oom=False, overflow=False, exitCode=1, signal=0,
                        cpuTimeMs=1, wallTimeMs=1, memoryBytes=1024,
                        output=base64.b64encode(b'partial\x00\xff').decode())
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', load_file=lambda _: '1 2\n')
        self.assertEqual(result['verdict'], 'RE')
        self.assertEqual(result['cases'][0]['sampleDetails'], dict(
            input=dict(text='1 2\n', truncated=False),
            expectedOutput=dict(text='', truncated=False),
            actualOutput=dict(text='partial��', truncated=False)))
        self.assertNotIn('output', result['cases'][0])

    def test_sample_preview_is_utf8_safe_and_queue_bounded(self):
        import json
        self.assertEqual(host.sample_preview('あい'.encode(), 4), dict(text='あ', truncated=True))
        limit = (24 * 1024) // 100 // 3
        preview = host.sample_preview(b'\x01' * 10000, limit)
        result = dict(cases=[dict(sampleDetails=dict(input=preview, expectedOutput=preview, actualOutput=preview)) for _ in range(100)])
        self.assertLess(len(json.dumps(result)), 200 * 1024)
        self.assertTrue(preview['truncated'])

    def test_validation_uses_exit_status_and_does_not_save_output(self):
        import tempfile
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   validate=True, memoryLimitMb=512, timeLimitMs=1000,
                   cases=[dict(input=str(i), output='secret') for i in range(3)])
        calls = []
        def execute(request, compile_phase=False):
            calls.append(compile_phase)
            if compile_phase:
                return {'compiled': True}
            index = int(base64.b64decode(request['input']))
            return dict(status='TO' if index == 2 else '', oom=False, overflow=False,
                        exitCode=1 if index == 1 else 0, signal=0,
                        cpuTimeMs=0, wallTimeMs=1, memoryBytes=1024, output=base64.b64encode(b'ignored').decode())
        save = Mock(side_effect=AssertionError('validation must not save output'))
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', save_output=save)
        self.assertEqual([c['verdict'] for c in result['cases']], ['AC', 'RE', 'TLE'])
        self.assertEqual(result['passed'], 1)
        self.assertEqual(calls, [True, False, False, False])
        self.assertTrue(all('output' not in c and 'outputFile' not in c for c in result['cases']))
        save.assert_not_called()
        job['validate'] = 'true'
        with self.assertRaises(ValueError):
            host.validate_job(job, 'sha256:test')

    def test_generation_preserves_stdout_and_enforces_file_and_total_limits(self):
        import tempfile
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   generate=True, memoryLimitMb=512, timeLimitMs=1000,
                   generationPrefix='test-files/' + 'a' * 32 + '/11111111-1111-4111-8111-111111111111/generated/',
                   cases=[dict(input='7\n', output=''), dict(input='8\n', output='')])
        for output, base, overflow, verdict in [
                (b' 3\n\n', 0, False, 'AC'), (b'', host.TEST_SET_LIMIT, False, 'AC'),
                (b'\xff', 0, False, 'RE'), (b'\0', 0, False, 'RE'),
                (b'x' * host.TEST_FILE_LIMIT, host.TEST_SET_LIMIT - 2 * host.TEST_FILE_LIMIT, False, 'AC'),
                (b'x' * host.TEST_FILE_LIMIT, host.TEST_SET_LIMIT - 2 * host.TEST_FILE_LIMIT + 1, False, 'OLE'),
                (b'x' * host.TEST_FILE_LIMIT, 0, True, 'OLE')]:
            calls, stored = [], []
            job['generationBaseBytes'] = base
            def execute(request, compile_phase=False):
                calls.append((request, compile_phase))
                if compile_phase:
                    return {'compiled': True}
                return dict(status='', oom=False, overflow=overflow, exitCode=0, signal=0,
                            cpuTimeMs=0, wallTimeMs=1, memoryBytes=1024,
                            output=base64.b64encode(output).decode())
            def save(data):
                stored.append(data)
                return {'size': len(data), 'id': str(len(stored))}
            with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                    patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                    patch.object(sandbox, 'META', Path(tmp) / 'meta'):
                result = host.judge(job, 'sha256:test', save_output=save)
            self.assertEqual(result['verdict'], verdict)
            self.assertEqual([phase for _, phase in calls], [True, False, False])
            self.assertEqual([base64.b64decode(req['input']) for req, phase in calls if not phase], [b'7\n', b'8\n'])
            if verdict == 'AC' and output:
                self.assertEqual(stored, [output, output])
                self.assertNotIn('output', result['cases'][0])
            elif verdict == 'AC':
                self.assertEqual(stored, [])
                self.assertEqual([c['output'] for c in result['cases']], ['', ''])
            else:
                self.assertNotIn('outputFile', result['cases'][-1])
                self.assertNotIn('output', result['cases'][-1])

    def test_generated_upload_is_checksummed_pending_and_versioned(self):
        import hashlib
        client = Mock()
        client.put_object.return_value = {'VersionId': 'immutable'}
        prefix = 'test-files/owner/problem/generated/'
        data = b'x' * host.TEST_FILE_LIMIT
        result = worker.write_generated_file(client, 'bucket', prefix, data)
        self.assertEqual(result['size'], host.TEST_FILE_LIMIT)
        self.assertEqual(result['sha256'], hashlib.sha256(data).hexdigest())
        self.assertEqual(result['versionId'], 'immutable')
        self.assertEqual(client.put_object.call_args.kwargs['Tagging'], 'status=pending')
        self.assertEqual(result['key'], prefix + result['id'])
        client.put_object.return_value = {}
        with self.assertRaises(ValueError):
            worker.write_generated_file(client, 'bucket', prefix, data)

    def test_compile_error_never_reports_judging(self):
        import tempfile
        progress = []
        job = dict(runtime='python314-isolate', runtimeDigest='sha256:test', source='bad syntax',
                   memoryLimitMb=512, timeLimitMs=1000, cases=[dict(input='', output='')])
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', return_value={'compiled': False, 'compileLog': 'syntax error'}), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', progress=lambda *args: progress.append(args))
        self.assertEqual(result['verdict'], 'CE')
        self.assertEqual(progress, [('PREPARING', 0, 1)])

    def test_large_test_file_is_loaded_by_immutable_reference(self):
        calls = []
        def execute(request, compile_phase=False):
            calls.append(request)
            if compile_phase:
                return {'compiled': True, 'compileLog': ''}
            return dict(status='', oom=False, overflow=False, exitCode=0, signal=0,
                        cpuTimeMs=1, wallTimeMs=1, memoryBytes=1024,
                        output=base64.b64encode(b'3\n').decode())
        file = dict(id='11111111-1111-4111-8111-111111111111', size=3, sha256='a' * 64,
                    key='test-files/' + 'b' * 32 + '/22222222-2222-4222-8222-222222222222/11111111-1111-4111-8111-111111111111',
                    versionId='version')
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   memoryLimitMb=512, timeLimitMs=1000,
                   cases=[dict(name='large', input='', output='', inputFile=file, outputFile=file)])
        import tempfile
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', lambda _: '3\n')
        self.assertEqual(result['verdict'], 'AC')
        self.assertEqual(calls[-1]['input'], base64.b64encode(b'3\n').decode())

    def test_validation_accepts_16_mib_input_file(self):
        import tempfile
        data = '1 ' * (host.TEST_FILE_LIMIT // 2)
        file = dict(id='11111111-1111-4111-8111-111111111111', size=len(data), sha256='a' * 64,
                    key='test-files/' + 'b' * 32 + '/22222222-2222-4222-8222-222222222222/11111111-1111-4111-8111-111111111111',
                    versionId='version')
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   validate=True, memoryLimitMb=512, timeLimitMs=5000,
                   cases=[dict(input='', output='', inputFile=file)])
        def execute(request, compile_phase=False):
            if compile_phase:
                return {'compiled': True}
            self.assertEqual(base64.b64decode(request['input']), data.encode())
            return dict(status='', oom=False, overflow=False, exitCode=0, signal=0,
                        cpuTimeMs=1, wallTimeMs=1, memoryBytes=1024, output='')
        load = Mock(return_value=data)
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', load_file=load)
        self.assertEqual(result['verdict'], 'AC')
        load.assert_called_once_with(file)
        job['cases'][0]['inputFile']['size'] += 1
        with self.assertRaises(ValueError):
            host.validate_job(job, 'sha256:test')

    def test_test_set_size_limit(self):
        file = dict(id='11111111-1111-4111-8111-111111111111', size=16 * 1024 * 1024,
                    sha256='a' * 64,
                    key='test-files/' + 'b' * 32 + '/22222222-2222-4222-8222-222222222222/11111111-1111-4111-8111-111111111111',
                    versionId='version')
        cases = [dict(input='', output='', inputFile=file, outputFile=file) for _ in range(16)]
        job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   memoryLimitMb=512, timeLimitMs=1000, cases=cases)
        host.validate_job(job, 'sha256:test')
        job['cases'] = cases + [dict(input='x', output='')]
        with self.assertRaisesRegex(ValueError, 'test set limit'):
            host.validate_job(job, 'sha256:test')

    def test_cleanup_failure_stops_worker(self):
        import subprocess
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            (Path(tmp) / 'box').mkdir()
            def invoke(args, **kwargs):
                if args == ['--init']:
                    return subprocess.CompletedProcess([], 0, (tmp+'\n').encode())
                if args == ['--cleanup']:
                    return subprocess.CompletedProcess([], 1)
                raise RuntimeError('simulated execution failure')
            with patch.object(sandbox, 'invoke', invoke), patch.object(sandbox, 'META', Path(tmp) / 'meta'):
                with self.assertRaises(SystemExit):
                    sandbox.execute({'source': 'int main(){}'}, True)

    def test_pointer_rejects_other_object_paths(self):
        import json
        item = dict(submissionId='11111111-1111-4111-8111-111111111111',
                    attemptId='22222222-2222-4222-8222-222222222222',
                    sha256='a' * 64, versionId='version')
        item['key'] = f"jobs/{item['submissionId']}/{item['attemptId']}.json"
        self.assertEqual(host.pointer(json.dumps(item)), item)
        item['key'] = 'releases/worker.tar.gz'
        with self.assertRaises(ValueError): host.pointer(json.dumps(item))

    def test_regular_read_rejects_symlink(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'output'
            path.symlink_to('/etc/passwd')
            with self.assertRaises(OSError): sandbox.regular_read(path, 10)


if __name__ == '__main__':
    unittest.main()
