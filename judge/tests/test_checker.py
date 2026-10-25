import base64
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from test_runner import host, sandbox, RUNTIMES


def reply(**changes):
    result = dict(status='', oom=False, overflow=False, exitCode=0, signal=0,
                  cpuTimeMs=1, wallTimeMs=2, memoryBytes=1024,
                  output=base64.b64encode(b'7\r\n').decode())
    result.update(changes)
    return result


def job():
    return dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='submitted source',
                checker=dict(runtime='python314', source='assert True'), memoryLimitMb=512,
                timeLimitMs=1000, cases=[dict(input='10\n', output='expected secret\n')] * 2)


class CheckerTests(unittest.TestCase):
    def run_job(self, checked, submitted=None, compiled=True, checker_runtime='python314', easy_test=False, protocol='legacy'):
        calls = []
        def execute(request, compile_phase=False, **kwargs):
            calls.append((request, compile_phase, kwargs))
            if compile_phase:
                Path(kwargs.get('artifact', sandbox.ARTIFACT)).write_bytes(request['source'].encode())
                return dict(compiled=compiled if kwargs else True, compileLog='private compiler log')
            if 'checker_files' in kwargs:
                self.assertEqual(Path(kwargs['artifact']).read_bytes(), b'assert True')
                return dict(checked)
            self.assertEqual(sandbox.ARTIFACT.read_bytes(), b'submitted source')
            return dict(submitted or reply())
        request = job()
        request['checker']['runtime'] = checker_runtime
        request['checker']['protocol'] = protocol
        request['easyTest'] = easy_test
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', execute), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'):
            result = host.judge(request, 'sha256:test')
            self.assertEqual(list(Path(tmp).iterdir()), [])
        return result, calls

    def test_independent_languages_artifacts_private_data_and_measurements(self):
        for runtime in RUNTIMES:
            if runtime == 'text-isolate':
                continue  # Text is not a judge-code language.
            with self.subTest(runtime=runtime):
                result, calls = self.run_job(reply(cpuTimeMs=4000, checkerLog='diagnostic'), checker_runtime=runtime.removesuffix('-isolate'))
                self.assertEqual(result['verdict'], 'AC')
                self.assertEqual(result['passed'], 2)
                self.assertEqual([c['cpuTimeMs'] for c in result['cases']], [1, 1])
                self.assertEqual(sum(compile for _, compile, _ in calls), 2)
                for request, compile, kwargs in calls:
                    if 'checker_files' in kwargs:
                        self.assertEqual(request['runtime'], runtime)
                        self.assertEqual(base64.b64decode(request['input']), b'7\r\n')
                        self.assertEqual(kwargs['checker_files'], {'test-input': b'10\n', 'expected-output': b'expected secret\n', 'submission-source': b'submitted source'})
                    elif not compile:
                        self.assertNotIn('expected secret', str((request, kwargs)))
                        self.assertNotIn('assert True', str((request, kwargs)))

    def test_sample_accepts_checker_verdict_despite_different_output(self):
        result, _ = self.run_job(reply(), easy_test=True)
        self.assertEqual(result['verdict'], 'AC')
        for case in result['cases']:
            self.assertEqual(case['verdict'], 'AC')
            self.assertEqual(case['sampleDetails']['expectedOutput']['text'], 'expected secret\n')
            self.assertEqual(case['sampleDetails']['actualOutput']['text'], '7\r\n')

    def test_assert_and_nonzero_are_wa_but_resource_failures_are_je(self):
        for changes, verdict in [({'exitCode': 1, 'status': 'RE'}, 'WA'),
                                 ({'signal': 6, 'status': 'SG'}, 'WA'),
                                 ({'exitCode': 100, 'status': 'RE'}, 'WA'),
                                 ({'status': 'TO'}, 'JE'), ({'oom': True}, 'JE'),
                                 ({'overflow': True}, 'JE')]:
            with self.subTest(changes=changes):
                result, _ = self.run_job(reply(**changes))
                self.assertEqual(result['verdict'], verdict)
                self.assertEqual(result['passed'], 0)
                if verdict == 'JE':
                    self.assertNotIn('cases', result)
        result, _ = self.run_job(reply(), compiled=False)
        self.assertEqual(result['verdict'], 'JE')
        self.assertIn('private compiler log', result['checkerLog'])
        self.assertNotIn('compileLog', result)

    def test_submission_failures_skip_checker(self):
        for changes, verdict in [({'exitCode': 1}, 'RE'), ({'status': 'TO'}, 'TLE'),
                                 ({'oom': True}, 'MLE'), ({'overflow': True}, 'OLE')]:
            result, calls = self.run_job(reply(), submitted=reply(**changes))
            self.assertEqual(result['verdict'], verdict)
            self.assertFalse(any('checker_files' in kwargs for _, _, kwargs in calls))

    def test_testlib_exit_codes_and_submission_priority(self):
        for changes, verdict in [({}, 'AC'), ({'exitCode': 1, 'status': 'RE'}, 'WA'),
                                 ({'exitCode': 2, 'status': 'RE'}, 'WA'), ({'exitCode': 4}, 'WA'),
                                 ({'exitCode': 8}, 'WA'), ({'exitCode': 3}, 'JE'),
                                 ({'exitCode': 7}, 'JE'), ({'exitCode': 99}, 'JE'),
                                 ({'signal': 6, 'status': 'SG'}, 'JE'), ({'oom': True}, 'JE')]:
            result, calls = self.run_job(reply(**changes), checker_runtime='cpp23-gcc', protocol='testlib')
            self.assertEqual(result['verdict'], verdict, changes)
            self.assertTrue(any(r.get('protocol') == 'testlib' for r, _, _ in calls))
        result, calls = self.run_job(reply(exitCode=3), submitted=reply(status='TO'), checker_runtime='cpp23-gcc', protocol='testlib')
        self.assertEqual(result['verdict'], 'TLE')
        self.assertFalse(any('checker_files' in kwargs for _, _, kwargs in calls))

    def test_testlib_file_contract_keeps_submission_bytes_and_secrets_separate(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            artifact = root / 'artifact'
            artifact.write_bytes(b'binary')
            for interactive in (False, True):
                box = root / str(interactive)
                box.mkdir()
                command = sandbox.prepare_program(box, RUNTIMES['cpp23-gcc-isolate'], artifact,
                    {'test-input': b'input', 'expected-output': b'answer', 'submission-source': b'private source',
                     'submission-output': b'out\x00\r\n'}, protocol='testlib', interactive=interactive)
                output = 'test-output' if interactive else 'submission-output'
                self.assertEqual(command[1:], ['/box/test-input', '/box/' + output, '/box/expected-output'])
                self.assertEqual((box / output).read_bytes(), b'' if interactive else b'out\x00\r\n')
                self.assertEqual((box / output).stat().st_mode & 0o777, 0o666 if interactive else 0o444)
                self.assertFalse((box / 'submission-source').exists())
                self.assertFalse((box / 'score').exists())
        for protocol, runtime in [('unknown', 'cpp23-gcc'), ('testlib', 'python314'), (None, 'cpp23-gcc')]:
            request = job()
            request['checker'].update(protocol=protocol, runtime=runtime)
            with self.assertRaises(ValueError): host.validate_job(request, 'sha256:test')

    def test_diagnostics_are_bounded_and_malformed_checkers_rejected(self):
        result, _ = self.run_job(reply(checkerLog='あ' * 65536))
        self.assertLessEqual(len(result['checkerLog'].encode()), 16384)
        for checker in [False, {}, {'runtime': 'sh', 'source': 'echo x'},
                        {'runtime': 'cpp23-gcc', 'source': ' '},
                        {'runtime': 'cpp23-gcc', 'source': 'a\0'},
                        {'runtime': 'cpp23-gcc', 'source': 'あ' * 22000}]:
            request = job()
            request['checker'] = checker
            with self.assertRaises(ValueError):
                host.validate_job(request, 'sha256:test')
        for mode in ['generate', 'validate']:
            request = job()
            request[mode] = True
            with self.assertRaises(ValueError):
                host.validate_job(request, 'sha256:test')

    def test_sandbox_uses_fixed_file_arguments_for_every_runtime(self):
        for name, runtime in RUNTIMES.items():
            with self.subTest(runtime=name), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                box = root / 'box'
                box.mkdir()
                artifact = root / 'checker'
                artifact.write_bytes(b'compiled checker')
                (root / 'dotnet-libs').mkdir()
                (root / 'deno-deps/cache').mkdir(parents=True)
                for filename in runtime.get('files', []):
                    (root / 'dotnet-libs' / filename).write_bytes(b'operator library')
                meta = root / 'meta'
                commands = []
                def invoke(args, timeout=10):
                    from subprocess import CompletedProcess
                    commands.append(args)
                    if '--run' in args:
                        program = runtime.get('program', 'main.dll' if runtime['artifact'] == 'dotnet' else 'main')
                        self.assertEqual((box / program).read_bytes(), b'compiled checker')
                        for filename in runtime.get('files', []):
                            self.assertEqual((box / filename).read_bytes(), b'operator library')
                            self.assertEqual((box / filename).stat().st_mode & 0o777, 0o444)
                        self.assertEqual((box / 'input').read_bytes(), b'actual\x00\r\n')
                        self.assertEqual((box / 'expected-output').read_bytes(), b'secret')
                        self.assertEqual((box / 'score').read_bytes(), b'')
                        self.assertEqual((box / 'score').stat().st_mode & 0o666, 0o666)
                        command = args[args.index('--') + 1:]
                        expected_command = [*runtime['run'], '/box/test-input', '/box/expected-output', '/box/submission-source', '/box/score']
                        if runtime['artifact'] == 'java':
                            expected_command.insert(1, '-ea')
                        self.assertEqual(command, expected_command)
                        meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                        (box / 'stdout').write_bytes(b'ignored')
                        (box / 'stderr').write_bytes(b'diagnostic')
                    return CompletedProcess(args, 0, stdout=str(root).encode())
                with patch.object(sandbox, 'invoke', invoke), patch.object(sandbox, 'META', meta), patch.object(sandbox, 'ROOT', str(root)):
                    result = sandbox.execute(dict(runtime=name, input=base64.b64encode(b'actual\x00\r\n').decode(), timeLimitMs=5000, memoryLimitMb=512),
                                             artifact=artifact, checker_files={'test-input': b'10', 'expected-output': b'secret', 'submission-source': b'source'})
                self.assertEqual(result['checkerLog'], 'diagnostic')
                self.assertEqual(commands[-1], ['--cleanup'])
