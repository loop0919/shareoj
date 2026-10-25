import base64
import sys
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'runner'))
import sandbox
import host
import worker
from runtimes import RUNTIMES


class RunnerTests(unittest.TestCase):
    def test_retired_runtimes_are_rejected_for_submissions_and_judges(self):
        for runtime in ('cpp17-isolate', 'java24-isolate'):
            for role in ('runtime', 'checker', 'interactor'):
                job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
                           memoryLimitMb=512, timeLimitMs=1000, cases=[dict(input='', output='')])
                job[role] = runtime if role == 'runtime' else dict(runtime=runtime.removesuffix('-isolate'), source='x')
                error = host.telemetry.PlatformError if role == 'runtime' else ValueError
                with self.subTest(runtime=runtime, role=role), self.assertRaises(error):
                    host.validate_job(job, 'sha256:test')

    def test_memory_limits_reach_isolate_and_reject_invalid_jobs(self):
        import tempfile
        from subprocess import CompletedProcess
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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

    def test_invalid_time_limits_are_rejected(self):
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   memoryLimitMb=512, cases=[dict(input='', output='')])
        for ms in (None, True, '1000', 1000.0, 0, 99, 5001, 5100, 150, 1050):
            with self.subTest(invalid=ms):
                job['timeLimitMs'] = ms
                with self.assertRaisesRegex(ValueError, 'time limit'):
                    host.validate_job(job, 'sha256:test')
        for ms in (100, 1000, 5000):
            with self.subTest(valid=ms):
                job['timeLimitMs'] = ms
                host.validate_job(job, 'sha256:test')

    def test_submission_heap_caps_scale_with_the_memory_limit(self):
        import tempfile
        from subprocess import CompletedProcess
        def flags(args, prefix):
            return [arg for arg in args if arg.startswith(prefix)]
        expected = {64: ('0x1800000', '48MiB', '--vm.Xmx32m', 40, '-Xmx32m'),
                    256: ('0x6000000', '192MiB', '--vm.Xmx128m', 160, '-Xmx128m'),
                    315: ('0x7600000', '236MiB', '--vm.Xmx157m', 196, '-Xmx157m'),
                    512: ('0xc000000', '384MiB', '--vm.Xmx256m', 320, '-Xmx256m')}
        for memory, (csharp, go, truffle, v8, java) in expected.items():
            environment = {'csharp14-isolate': ('--env=DOTNET_GCHeapHardLimit=', [csharp]),
                           'go127-isolate': ('--env=GOMEMLIMIT=', [go]),
                           'ruby-truffle40-isolate': ('--env=RUBYOPT=', [truffle]),
                           'javascript-node24-isolate': ('--env=NODE_OPTIONS=', [f'--max-old-space-size={v8}']),
                           'typescript-node24-isolate': ('--env=NODE_OPTIONS=', [f'--max-old-space-size={v8}']),
                           'javascript-deno29-isolate': ('--env=DENO_V8_FLAGS=', [f'--max-old-space-size={v8}']),
                           'typescript-deno29-isolate': ('--env=DENO_V8_FLAGS=', [f'--max-old-space-size={v8}']),
                           # Bun gets NODE_OPTIONS only while compiling.
                           'typescript-bun14-isolate': ('--env=NODE_OPTIONS=', [])}
            for name, (prefix, values) in environment.items():
                with self.subTest(memory=memory, runtime=name):
                    self.assertEqual(flags(sandbox.run_args(name, 1, 4, memory), prefix), [prefix + value for value in values])
            for name in ('java25-isolate',):
                with self.subTest(memory=memory, runtime=name), tempfile.TemporaryDirectory() as tmp:
                    root = Path(tmp)
                    (root / 'box').mkdir()
                    artifact, meta = root / 'main', root / 'meta'
                    artifact.write_bytes(b'program')
                    commands = []
                    def invoke(args, **kwargs):
                        if '--run' in args:
                            commands.append(args[args.index('--') + 1:])
                            meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                            (root / 'box/stdout').write_bytes(b'')
                            (root / 'box/stderr').write_bytes(b'')
                        return CompletedProcess(args, 0, stdout=str(root).encode())
                    with patch.object(sandbox, 'invoke', invoke), patch.object(sandbox, 'META', meta):
                        sandbox.execute(dict(runtime=name, input='', timeLimitMs=1000, memoryLimitMb=memory), artifact=artifact)
                    self.assertEqual([flags(command, '-Xm') for command in commands], [['-Xms16m', java]])

    def test_compile_heap_caps_stay_fixed(self):
        expected = {'csharp14-isolate': 'DOTNET_GCHeapHardLimit=0x20000000',
                    'go127-isolate': 'GOMEMLIMIT=384MiB',
                    'ruby-truffle40-isolate': 'RUBYOPT=--vm.Xmx640m',
                    'javascript-node24-isolate': 'NODE_OPTIONS=--max-old-space-size=640',
                    'typescript-node24-isolate': 'NODE_OPTIONS=--max-old-space-size=640',
                    'typescript-bun14-isolate': 'NODE_OPTIONS=--max-old-space-size=640',
                    'javascript-deno29-isolate': 'DENO_V8_FLAGS=--max-old-space-size=640',
                    'typescript-deno29-isolate': 'DENO_V8_FLAGS=--max-old-space-size=640'}
        for name, value in expected.items():
            with self.subTest(runtime=name):
                self.assertIn('--env=' + value, sandbox.run_args(name, 30, 40, 1024, compile_phase=True))

    def test_two_tles_skip_remaining_cases_only_for_normal_submissions(self):
        import tempfile
        for mode in (None, 'easyTest', 'validate', 'generate', 'interactive'):
            for verdicts in (['TLE', 'AC', 'TLE', 'AC'], ['TLE', 'TLE', 'AC', 'AC'],
                             ['RE', 'TLE', 'TLE', 'AC'], ['TLE', 'AC', 'AC', 'AC']):
                with self.subTest(mode=mode, verdicts=verdicts):
                    job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
                               memoryLimitMb=512, timeLimitMs=1000,
                               cases=[dict(input=str(i), output='') for i in range(4)])
                    if mode == 'interactive':
                        job['interactor'] = dict(runtime='cpp23-gcc', source='int main(){}')
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
                    def interact(job, case, artifact, diagnostic, **kwargs):
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
        fixtures = json.loads((Path(__file__).resolve().parents[1] / 'runner/language-smoke.json').read_text())
        self.assertEqual(set(fixtures), set(RUNTIMES))
        for name, runtime in RUNTIMES.items():
            self.assertTrue(runtime['compile'][0].startswith(('/opt/judge-runtimes/', '/usr/bin/')))
            self.assertTrue(runtime['run'][0] == '/usr/bin/sh' or runtime['run'][0].startswith(('/opt/judge-runtimes/', '/box/')))
            required = {'AC', 'WA', 'TLE'}
            if runtime['compile'] != ['/usr/bin/true']:
                required.add('CE')
            self.assertTrue(required <= {case['verdict'] for case in fixtures[name]}, name)

    def test_java_artifact_rejects_symlink_directory(self):
        import tempfile
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp)
            (box / 'classes').mkdir()
            (box / 'classes' / 'escape').symlink_to('/etc', target_is_directory=True)
            with self.assertRaisesRegex(ValueError, 'class directory'):
                sandbox.collect_artifact(box, RUNTIMES['java25-isolate'])

    def test_java_artifact_preserves_nested_classes(self):
        import tempfile
        import zipfile
        import io
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp)
            (box / 'classes' / 'nested').mkdir(parents=True)
            (box / 'classes' / 'Main.class').write_bytes(b'main')
            (box / 'classes' / 'nested' / 'Helper.class').write_bytes(b'helper')
            with zipfile.ZipFile(io.BytesIO(sandbox.collect_artifact(box, RUNTIMES['java25-isolate']))) as jar:
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

    def test_case_failure_flags_follow_ole_mle_tle_re_priority(self):
        def reply(**changes):
            return dict(index=0, status='', oom=False, overflow=False, exitCode=0, signal=0,
                        cpuTimeMs=1, wallTimeMs=2, memoryBytes=4096, output='') | changes
        # Every output differs from the expected one, so a failure judged after the comparison would read WA.
        for overflow, oom, status, exit_code, signal, expected in [
                (True, True, 'TO', 1, 9, 'OLE'), (True, False, '', 0, 0, 'OLE'),
                (False, True, 'TO', 1, 9, 'MLE'), (False, True, '', 0, 0, 'MLE'),
                (False, False, 'TO', 1, 9, 'TLE'), (False, False, 'TO', 0, 0, 'TLE'),
                (False, False, 'SG', 0, 9, 'RE'), (False, False, 'RE', 1, 0, 'RE'),
                (False, False, '', 1, 0, 'RE'), (False, False, '', 0, 9, 'RE'),
                (False, False, 'RE', 0, 0, 'RE'), (False, False, 'SG', 0, 0, 'RE'),
                (False, False, '', 0, 0, 'WA')]:
            with self.subTest(overflow=overflow, oom=oom, status=status, exitCode=exit_code, signal=signal):
                result = host.case_result(reply(overflow=overflow, oom=oom, status=status, exitCode=exit_code, signal=signal),
                                          0, {'output': 'expected'})
                self.assertEqual(result['verdict'], expected)
        self.assertEqual(host.case_result(reply(), 0, {'output': ' \n'})['verdict'], 'AC')
        # A failed generation keeps its verdict and does not expose the output.
        self.assertEqual(host.case_result(reply(status='TO', output=base64.b64encode(b'\0').decode()), 0, {'output': ''}, generate=True),
                         {'name': 'ケース1', 'verdict': 'TLE', 'cpuTimeMs': 1, 'wallTimeMs': 2, 'memoryBytes': 4096})

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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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

    def test_case_data_uses_lf_and_ends_with_a_newline_unless_empty(self):
        import tempfile
        file = dict(id='33333333-3333-4333-8333-333333333333', size=5, sha256='a' * 64,
                    key='test-files/' + 'a' * 32 + '/33333333-3333-4333-8333-333333333333/33333333-3333-4333-8333-333333333333',
                    versionId='v')
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   easyTest=True, memoryLimitMb=512, timeLimitMs=1000,
                   cases=[dict(input='abc\r\nd\re', output='3'),
                          dict(input='', output='', inputFile=file, outputFile=file),
                          dict(input='', output='')])
        inputs = []
        def execute(request, compile_phase=False):
            if compile_phase:
                return dict(compiled=True)
            inputs.append(base64.b64decode(request['input']))
            return dict(status='', oom=False, overflow=False, exitCode=0, signal=0,
                        cpuTimeMs=1, wallTimeMs=1, memoryBytes=1024, output='')
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(job, 'sha256:test', load_file=lambda _: 'x\r\ny\r')
        self.assertEqual(inputs, [b'abc\nd\ne\n', b'x\ny\n', b''])
        self.assertEqual([c['sampleDetails']['expectedOutput']['text'] for c in result['cases']], ['3\n', 'x\ny\n', ''])

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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
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
        job = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   memoryLimitMb=512, timeLimitMs=1000, cases=cases)
        host.validate_job(job, 'sha256:test')
        job['cases'] = cases + [dict(input='x', output='')]
        with self.assertRaisesRegex(ValueError, 'test set limit'):
            host.validate_job(job, 'sha256:test')

    def test_source_size_limit(self):
        job = dict(runtime='python314-isolate', runtimeDigest='sha256:test', source='#' * 131072,
                   memoryLimitMb=512, timeLimitMs=1000, cases=[dict(input='', output='')])
        host.validate_job(job, 'sha256:test')
        job['source'] = 'あ' * 43691
        with self.assertRaisesRegex(ValueError, 'source'):
            host.validate_job(job, 'sha256:test')
        with self.assertRaisesRegex(ValueError, 'invalid source'):
            sandbox.execute(job, compile_phase=True)

    def test_whitespace_judge_source_survives_validation_and_artifact_copy(self):
        import tempfile
        source = '  \t \n\t\n \t\n\n\n'
        code = dict(runtime='whitespace13', source=source)
        host.validate_judge_code(code)
        for invalid in ('', '\0', ' ' * 65537):
            with self.assertRaises(ValueError):
                host.validate_judge_code(dict(code, source=invalid))
        with self.assertRaises(ValueError):
            host.validate_judge_code(dict(code, runtime='cpp23-gcc'))
        runtime = RUNTIMES['whitespace13-isolate']
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp)
            (box / 'main.ws').write_text(source)
            data = sandbox.collect_artifact(box, runtime)
            self.assertEqual(data, source.encode())
            artifact = box / 'artifact'
            artifact.write_bytes(data)
            sandbox.prepare_program(box, runtime, artifact)
            self.assertEqual((box / 'main.ws').read_bytes(), source.encode())

    def test_one_source_limit_covers_validation_and_reading_back_the_source(self):
        import tempfile
        self.assertEqual(sandbox.SOURCE_LIMIT, 131072)
        runtime = RUNTIMES['python314-isolate']
        for limit in (8, 200000):
            with self.subTest(limit=limit), patch.object(sandbox, 'SOURCE_LIMIT', limit), \
                    tempfile.TemporaryDirectory() as tmp:
                job = dict(runtime='python314-isolate', runtimeDigest='sha256:test', source='#' * limit,
                           memoryLimitMb=512, timeLimitMs=1000, cases=[dict(input='', output='')])
                host.validate_job(job, 'sha256:test')
                (Path(tmp) / 'main.py').write_text(job['source'])
                self.assertEqual(sandbox.collect_artifact(Path(tmp), runtime), job['source'].encode())
                job['source'] += '#'
                with self.assertRaisesRegex(ValueError, 'source'):
                    host.validate_job(job, 'sha256:test')
                with patch.object(sandbox, 'invoke') as invoke, self.assertRaisesRegex(ValueError, 'invalid source'):
                    sandbox.execute(job, compile_phase=True)
                invoke.assert_not_called()

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

    def code_run_job(self, **changes):
        job = dict(codeRun=True, runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='int main(){}',
                   stdin=base64.b64encode(b'1 2\n').decode(), timeLimitMs=5000, memoryLimitMb=512)
        return dict(job, **changes)

    def run_code(self, reply=None, compiled=None, job=None):
        import tempfile
        b64 = lambda data: base64.b64encode(data).decode()
        reply = dict(dict(status='', oom=False, overflow=False, signal=0, exitCode=0, cpuTimeMs=12, wallTimeMs=15,
                          memoryBytes=3600000, output=b64(b'3\n'), checkerLog='debug\n'), **(reply or {}))
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'), \
                patch.object(sandbox, 'execute', side_effect=[compiled or {'compiled': True}, reply]) as execute:
            result = host.run_code(job or self.code_run_job(), 'sha256:test')
        return result, execute

    def test_code_run_returns_output_stderr_and_measurements(self):
        result, execute = self.run_code()
        self.assertEqual(result, {'verdict': 'AC', 'exitCode': 0, 'cpuTimeMs': 12, 'wallTimeMs': 15, 'memoryBytes': 3600000,
                                  'stdout': {'text': '3\n', 'truncated': False}, 'stderr': {'text': 'debug\n', 'truncated': False}})
        self.assertEqual(execute.call_count, 2)
        self.assertTrue(execute.call_args_list[0].args[1])
        run = execute.call_args_list[1]
        self.assertEqual(run.args[0]['input'], base64.b64encode(b'1 2\n').decode())
        self.assertEqual((run.args[0]['timeLimitMs'], run.args[0]['memoryLimitMb']), (5000, 512))
        self.assertIs(run.kwargs['stderr_log'], True)

    def test_code_run_maps_failures_like_submissions(self):
        for reply, verdict in ((dict(exitCode=3), 'RE'), (dict(status='TO'), 'TLE'), (dict(oom=True), 'MLE'), (dict(overflow=True), 'OLE')):
            with self.subTest(verdict=verdict):
                result, _ = self.run_code(reply)
                self.assertEqual(result['verdict'], verdict)
                self.assertEqual(result['exitCode'], reply.get('exitCode', 0))

    def test_code_run_compile_error_has_only_the_log(self):
        result, execute = self.run_code(compiled={'compiled': False, 'compileLog': 'error'})
        self.assertEqual(result, {'verdict': 'CE', 'compileLog': 'error'})
        self.assertEqual(execute.call_count, 1)

    def test_code_run_truncates_long_output(self):
        result, _ = self.run_code(dict(output=base64.b64encode(b'x' * 70000).decode()))
        self.assertEqual(len(result['stdout']['text']), 65536)
        self.assertTrue(result['stdout']['truncated'])

    def test_code_run_shrinks_control_character_output_to_fit_the_queue(self):
        import json
        result, _ = self.run_code(dict(output=base64.b64encode(b'\x01' * 65536).decode(), checkerLog='\x01' * 65536))
        self.assertLessEqual(len(json.dumps(result, ensure_ascii=False, allow_nan=False).encode()), 240 * 1024)
        self.assertEqual(result['verdict'], 'AC')
        for name in ('stdout', 'stderr'):
            self.assertLessEqual(len(result[name]['text']), 16384)
            self.assertTrue(result[name]['truncated'])

    def test_code_run_rejects_invalid_jobs(self):
        b64 = lambda data: base64.b64encode(data).decode()
        invalid = [dict(timeLimitMs=1000), dict(memoryLimitMb=256), dict(stdin='***'), dict(stdin=b64(b'x' * 1048577)),
                   dict(stdin=b64(b'a\0b')), dict(cases=[]), dict(checker=dict(runtime='cpp23-gcc', source='x')), dict(generate=True)]
        for changes in invalid:
            with self.subTest(changes=list(changes)), patch.object(sandbox, 'execute') as execute:
                with self.assertRaises(ValueError):
                    host.validate_code_run(self.code_run_job(**changes), 'sha256:test')
                execute.assert_not_called()
        host.validate_code_run(self.code_run_job(stdin=b64(b'x' * 1048576)), 'sha256:test')
        with self.assertRaises(host.telemetry.PlatformError):
            host.validate_code_run(self.code_run_job(runtimeDigest='sha256:other'), 'sha256:test')


if __name__ == '__main__':
    unittest.main()
