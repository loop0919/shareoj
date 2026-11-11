import base64
import os
import subprocess
import tempfile
from pathlib import Path
import unittest
from unittest.mock import Mock, patch

from test_runner import host  # noqa: F401  (puts judge/runner on sys.path)
import interactive
import sandbox
from runtimes import RUNTIMES

FILES = {'test-input': b'in', 'expected-output': b'out', 'submission-source': b'src'}


def job(**extra):
    return dict(runtime='cpp23-gcc-isolate', source='s', memoryLimitMb=256, timeLimitMs=1000,
                interactor=dict(runtime='python314', source='x'),
                runTwice=dict(runtime='python314', source='y'), **extra)


class RunTwiceTests(unittest.TestCase):
    def prepare(self, **kwargs):
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp) / 'box'
            box.mkdir()
            artifact = Path(tmp) / 'artifact'
            artifact.write_bytes(b'bin')
            command = sandbox.prepare_program(box, RUNTIMES['cpp23-gcc-isolate'], artifact, **kwargs)
            contents = {p.name: (p.read_bytes(), p.stat().st_mode & 0o777) for p in box.iterdir()}
        return command, contents

    def test_legacy_arguments_and_handoff_permissions(self):
        command, contents = self.prepare(files=FILES, interactive=True, run=1)
        self.assertEqual(command[-2:], ['1', '/box/handoff'])
        self.assertEqual(contents['handoff'], (b'', 0o666))
        command, contents = self.prepare(files=FILES, interactive=True, run=2, handoff=b'x')
        self.assertEqual(command[-2:], ['2', '/box/handoff'])
        self.assertEqual(contents['handoff'], (b'x', 0o444))
        command, contents = self.prepare(files=FILES, interactive=True)
        self.assertEqual(command[-4:], ['/box/test-input', '/box/expected-output', '/box/submission-source', '/box/score'])
        self.assertNotIn('handoff', contents)

    def test_testlib_second_run_reads_the_handoff_as_input(self):
        command, contents = self.prepare(files=FILES, protocol='testlib', interactive=True, run=2, handoff=b'1 3\n')
        self.assertEqual(contents['test-input'][0], b'1 3\n')
        self.assertEqual(command[-3:], ['/box/test-input', '/box/test-output', '/box/expected-output'])
        self.assertNotIn('handoff', contents)
        _, contents = self.prepare(files=FILES, protocol='testlib', interactive=True, run=1)
        self.assertEqual(contents['test-input'][0], b'in')

    def two_runs(self, first, second=None, score_file=False):
        results = [first] + ([second] if second else [])
        calls, diagnostics = [], []

        def execute(request, case, artifact, diagnostic, score_file=False, **kwargs):
            calls.append(kwargs)
            diagnostic('履歴%d\n' % len(calls))
            return results[len(calls) - 1]
        with patch.object(interactive, 'execute', execute):
            result = interactive.execute_twice(job(), dict(input='i', output='o'), Path('a'), diagnostics.append, score_file)
        return result, calls, ''.join(diagnostics)

    def test_second_run_receives_what_the_first_run_wrote(self):
        result, calls, _ = self.two_runs(
            dict(verdict='AC', handoff=b'h', cpuTimeMs=5, wallTimeMs=10, memoryBytes=100),
            dict(verdict='AC', cpuTimeMs=3, wallTimeMs=7, memoryBytes=200))
        self.assertEqual(calls, [dict(code=job()['runTwice'], run=1), dict(code=job()['runTwice'], run=2, handoff=b'h')])
        self.assertEqual(result, dict(verdict='AC', cpuTimeMs=5, wallTimeMs=17, memoryBytes=200))

    def test_first_run_failure_skips_the_second_run(self):
        for verdict in ('WA', 'TLE', 'RE', 'JE'):
            with self.subTest(verdict=verdict):
                result, calls, text = self.two_runs(dict(verdict=verdict, cpuTimeMs=5, wallTimeMs=10, memoryBytes=100))
                self.assertEqual(len(calls), 1)
                self.assertEqual(result, dict(verdict=verdict, cpuTimeMs=5, wallTimeMs=10, memoryBytes=100))
                self.assertIn(f'2回目: 実行しません（1回目の判定: {verdict}）\n', text)

    def test_empty_handoff_still_runs_the_second_run(self):
        _, calls, _ = self.two_runs(dict(verdict='AC', handoff=b'', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1),
                                    dict(verdict='AC', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1))
        self.assertEqual(len(calls), 2)
        self.assertEqual(calls[1]['handoff'], b'')

    def test_unreadable_handoff_is_je(self):
        result, calls, _ = self.two_runs(dict(verdict='AC', handoff=None, cpuTimeMs=1, wallTimeMs=1, memoryBytes=1))
        self.assertEqual(len(calls), 1)
        self.assertEqual(result['verdict'], 'JE')
        self.assertIn('16 MiB', result['reason'])

    def test_points_come_from_the_second_run_only(self):
        result, _, _ = self.two_runs(
            dict(verdict='AC', handoff=b'', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1, scoreFile=b'50', judgeExitCode=0),
            dict(verdict='AC', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1, scoreFile=b'80', judgeExitCode=0), score_file=True)
        self.assertEqual((result['scoreFile'], result['judgeExitCode']), (b'80', 0))

    def test_headings_separate_the_two_runs(self):
        _, _, text = self.two_runs(dict(verdict='AC', handoff=b'', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1),
                                   dict(verdict='AC', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1))
        self.assertEqual(text, '1回目:\n履歴1\n2回目:\n履歴2\n')

    def execute_first_run(self, protocol, plant):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for i in range(2):
                (root / str(i) / 'box').mkdir(parents=True)

            def invoke(args, *, box_id=0, **kwargs):
                return subprocess.CompletedProcess(args, 0, stdout=str(root / str(box_id)).encode())

            def popen(command, **kwargs):
                meta = Path(next(arg.removeprefix('--meta=') for arg in command if arg.startswith('--meta=')))
                meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                return Mock(returncode=0, poll=Mock(return_value=0))

            def prepare(box, *args, **kwargs):
                if box == root / '1' / 'box':
                    plant(box)
                return ['/box/main']
            request = job()
            request['runTwice']['protocol'] = protocol
            with patch.object(interactive.sandbox, 'invoke', invoke), \
                    patch.object(interactive.sandbox, 'META', root / 'meta'), \
                    patch.object(interactive.sandbox, 'prepare_program', prepare), \
                    patch.object(interactive.subprocess, 'Popen', popen), \
                    patch.object(interactive, 'relay', return_value='AC'):
                return interactive.execute(request, dict(input='i', output='o'), root / 'checker', lambda _: None,
                                           code=request['runTwice'], run=1)

    def test_handoff_that_is_not_a_regular_file_is_je(self):
        def write(name, data):
            return lambda box: (box / name).write_bytes(data)

        def link(box):
            (box / 'target').write_bytes(b'x')
            os.symlink(box / 'target', box / 'handoff')
        self.assertEqual(self.execute_first_run('legacy', write('handoff', b'ok'))['handoff'], b'ok')
        self.assertEqual(self.execute_first_run('testlib', write('test-output', b'ok'))['handoff'], b'ok')
        self.assertIsNone(self.execute_first_run('legacy', link)['handoff'])
        self.assertIsNone(self.execute_first_run('legacy', lambda box: None)['handoff'])
        self.assertIsNone(self.execute_first_run('legacy', write('handoff', b'x' * (sandbox.HANDOFF_LIMIT + 1)))['handoff'])
        self.assertEqual(len(self.execute_first_run('legacy', write('handoff', b'x' * sandbox.HANDOFF_LIMIT))['handoff']),
                         sandbox.HANDOFF_LIMIT)

    def test_score_file_of_a_skipped_second_run_is_empty_for_legacy_only(self):
        for protocol, expected in (('legacy', b''), ('testlib', b'50')):
            with self.subTest(protocol=protocol):
                request = job()
                request['runTwice']['protocol'] = protocol
                first = dict(verdict='WA', cpuTimeMs=1, wallTimeMs=1, memoryBytes=1, scoreFile=b'50', judgeExitCode=1)
                with patch.object(interactive, 'execute', lambda *args, **kwargs: dict(first)):
                    result = interactive.execute_twice(request, dict(input='i', output='o'), Path('a'), lambda _: None, True)
                self.assertEqual((result['scoreFile'], result['judgeExitCode']), (expected, 1))


def twice_job(**changes):
    request = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='submitted source',
                   runTwice=dict(runtime='python314', source='assert True'), memoryLimitMb=512, timeLimitMs=1000,
                   cases=[dict(input='10\n', output='expected\n')] * 3)
    request.update(changes)
    return request


def ok(**changes):
    return dict(verdict='AC', cpuTimeMs=7, wallTimeMs=20, memoryBytes=512, **changes)


class RunTwiceHostTests(unittest.TestCase):
    def judge(self, request, twice, compile_ok=True):
        compiled = []

        def compile(item, compile_phase=False, **kwargs):
            compiled.append(item['runtime'])
            Path(kwargs.get('artifact', host.sandbox.ARTIFACT)).write_text(item['source'])
            return dict(compiled=compile_ok or len(compiled) < 2, compileLog='log')
        with tempfile.TemporaryDirectory() as tmp, patch.object(host.sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(host.sandbox, 'META', Path(tmp) / 'meta'), \
                patch.object(host.sandbox, 'execute', compile), \
                patch.object(host.telemetry, 'note_failure') as note, \
                patch.object(interactive, 'execute_twice', twice) as run:
            result = host.judge(request, 'sha256:test')
        return result, compiled, note, run

    def test_jobs_reject_two_judge_codes(self):
        code = dict(runtime='python314', source='x')
        for others in (('checker',), ('interactor',), ('checker', 'interactor')):
            request = twice_job(**{name: code for name in others})
            with self.assertRaises(ValueError):
                host.validate_job(request, 'sha256:test')
        host.validate_job(twice_job(), 'sha256:test')
        for field in ('generate', 'validate'):
            with self.assertRaises(ValueError):
                host.validate_job(twice_job(**{field: True}), 'sha256:test')
        for bad in ({}, False, {'runtime': 'sh', 'source': 'x'}, {'runtime': 'python314', 'source': ''}):
            with self.assertRaises(ValueError):
                host.validate_job(twice_job(runTwice=bad), 'sha256:test')

    def test_host_runs_each_case_through_execute_twice(self):
        metrics = [(1, 2, 3), (4, 5, 6), (7, 8, 9)]
        calls = iter(metrics)

        def twice(request, case, artifact, diagnostic, score_file=False):
            cpu, wall, memory = next(calls)
            return dict(verdict='AC', cpuTimeMs=cpu, wallTimeMs=wall, memoryBytes=memory)
        result, compiled, _, _ = self.judge(twice_job(), twice)
        self.assertEqual(compiled, ['cpp23-gcc-isolate', 'python314-isolate'])
        self.assertEqual(result['verdict'], 'AC')
        self.assertEqual(result['passed'], 3)
        self.assertEqual([(c['cpuTimeMs'], c['wallTimeMs'], c['memoryBytes']) for c in result['cases']], metrics)

    def test_judge_compile_failure_is_reported(self):
        result, _, note, _ = self.judge(twice_job(), None, compile_ok=False)
        self.assertEqual(result['verdict'], 'JE')
        self.assertEqual(note.call_args.args, ('judge_code', 'run_twice_compile_failed'))

    def test_handoff_failure_makes_the_submission_je(self):
        reason = '引き継ぎファイルを読めません（16 MiB以下の通常のファイルにしてください）\n'
        result, _, note, _ = self.judge(twice_job(), lambda *args, **kwargs: dict(verdict='JE', reason=reason))
        self.assertEqual(result['verdict'], 'JE')
        self.assertTrue(result['checkerLog'].endswith(reason))
        self.assertEqual(note.call_args.args, ('judge_code', 'run_twice_execution_failed'))

    def test_first_run_tle_counts_once_per_case_for_group_cutoff(self):
        from test_scoring import scoring_job
        request = scoring_job(groups=(('a', 100),), members=(['a'], ['a'], ['a']), runTwice=dict(runtime='python314', source='x'))
        calls = []

        def twice(request, case, artifact, diagnostic, score_file=False):
            calls.append(case['input'])
            return ok() | dict(verdict='TLE')
        result, _, _, _ = self.judge(request, twice)
        self.assertEqual(len(calls), 2)
        self.assertEqual([c['verdict'] for c in result['cases']], ['TLE', 'TLE', 'SKIPPED'])

    def test_aggregate_points_use_the_returned_score_file(self):
        from test_scoring import scoring_job, finished
        request = scoring_job('aggregate', groups=(('a', 100),), members=(['a'],), runTwice=dict(runtime='python314', source='x'))
        seen = []

        def compile(item, compile_phase=False, **kwargs):
            if compile_phase:
                Path(kwargs.get('artifact', host.sandbox.ARTIFACT)).write_text(item['source'])
                return dict(compiled=True)
            seen.append(base64.b64decode(item['input']).decode())
            return finished(output=base64.b64encode(b'80').decode(), checkerLog='')
        twice = Mock(return_value=ok(scoreFile=b'80', judgeExitCode=0))
        with tempfile.TemporaryDirectory() as tmp, patch.object(host.sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(host.sandbox, 'META', Path(tmp) / 'meta'), \
                patch.object(host.sandbox, 'execute', compile), patch.object(interactive, 'execute_twice', twice):
            result = host.judge(request, 'sha256:test')
        self.assertEqual(result['cases'][0]['score'], 80)
        self.assertTrue(twice.call_args.kwargs['score_file'])
        self.assertEqual(seen, ['1 1\n100\nAC 80 0 1 0\n'])

    def test_sample_check_keeps_per_case_log_with_headings(self):
        def twice(request, case, artifact, diagnostic, score_file=False):
            diagnostic('1回目:\n履歴\n')
            diagnostic('2回目: 実行しません（1回目の判定: WA）\n')
            return ok() | dict(verdict='WA')
        result, _, _, _ = self.judge(twice_job(easyTest=True), twice)
        for case in result['cases']:
            self.assertNotIn('sampleDetails', case)
            self.assertIn('1回目:\n履歴\n2回目: 実行しません（1回目の判定: WA）\n', case['checkerLog']['text'])

    def test_sample_log_keeps_the_second_run_after_a_long_first_run(self):
        skipped = '2回目: 実行しません（1回目の判定: WA）\n'

        def dialogue(diagnostic):
            diagnostic('1回目:\n')
            diagnostic('提出 → ジャッジ: ' + 'あ' * 2000 + '\n')
            diagnostic(skipped)
        request = twice_job(easyTest=True, cases=[dict(input='1\n', output='1\n')])
        result, _, _, _ = self.judge(request, lambda *args, **kwargs: dialogue(args[3]) or ok() | dict(verdict='WA'))
        log = result['cases'][0]['checkerLog']
        self.assertTrue(log['truncated'])
        self.assertTrue(log['text'].startswith('1回目:\n提出 → ジャッジ: あ'))
        self.assertTrue(log['text'].endswith('\n' + skipped), log['text'][-80:])
        self.assertLessEqual(len(log['text'].encode()), 4096)
        # An interactor keeps one budget for the whole case, cut from the head as before.
        from test_scoring import Judge
        request['interactor'] = request.pop('runTwice')
        interactor = Judge(self)
        messages = []
        interactor.interact = lambda job, case, artifact, diagnostic, score_file=False: dialogue(
            lambda message: messages.append(message) or diagnostic(message)) or ok() | dict(verdict='WA')
        log = interactor(request)['cases'][0]['checkerLog']
        self.assertEqual(log, host.sample_preview(''.join(messages).encode()[:4097], 4096))
        self.assertNotIn('2回目', log['text'])

    def test_results_of_a_run_twice_job_carry_the_marker(self):
        from test_scoring import Judge, finished
        for verdict in ('AC', 'WA'):
            with self.subTest(verdict=verdict):
                result, _, _, _ = self.judge(twice_job(), lambda *args, **kwargs: ok() | dict(verdict=verdict))
                self.assertEqual((result['verdict'], result.get('runTwice')), (verdict, True))
        result = Judge(self, compiled={'submitted source': False})(twice_job())
        self.assertEqual((result['verdict'], result.get('runTwice')), ('CE', True))
        # Plain, checker and interactor jobs keep their result shape.
        plain = twice_job(cases=[dict(input='1', output='')])
        del plain['runTwice']
        code = dict(runtime='python314', source='x')
        for request in (plain, dict(plain, checker=code), dict(plain, interactor=code)):
            for compiled in ({}, {'submitted source': False}):
                with self.subTest(job=sorted(request), compiled=compiled):
                    result = Judge(self, checker=lambda _: finished(), interactor=lambda _: ok(), compiled=compiled)(request)
                    self.assertEqual(result['verdict'], 'CE' if compiled else 'AC')
                    self.assertNotIn('runTwice', result)

    def test_local_runner_rejects_run_twice_jobs(self):
        import io
        import json
        import importlib.util
        spec = importlib.util.spec_from_file_location('local_runner', Path(__file__).resolve().parents[1] / 'local.py')
        local = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(local)
        with patch('sys.stdin', io.StringIO(json.dumps(twice_job()) + '\n')), self.assertRaisesRegex(ValueError, '2ステップ形式'):
            local.main()


if __name__ == '__main__':
    unittest.main()
