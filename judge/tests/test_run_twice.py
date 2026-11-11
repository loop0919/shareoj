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


if __name__ == '__main__':
    unittest.main()
