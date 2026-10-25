import subprocess
import sys
import tempfile
from pathlib import Path
import time
import unittest
from unittest.mock import Mock, patch

from test_runner import host
import interactive


class InteractiveTests(unittest.TestCase):
    def execute_stubbed(self, request, **prepare_program):
        commands = []
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for i in range(2):
                (root / str(i) / 'box').mkdir(parents=True)
            def invoke(args, *, box_id=0, **kwargs):
                return subprocess.CompletedProcess(args, 0, stdout=str(root / str(box_id)).encode())
            def popen(command, **kwargs):
                commands.append(command)
                meta = Path(next(arg.removeprefix('--meta=') for arg in command if arg.startswith('--meta=')))
                meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                return Mock(returncode=0, poll=Mock(return_value=0))
            with patch.object(interactive.sandbox, 'invoke', invoke), \
                    patch.object(interactive.sandbox, 'META', root / 'meta'), \
                    patch.object(interactive.sandbox, 'prepare_program', **prepare_program), \
                    patch.object(interactive.subprocess, 'Popen', popen), \
                    patch.object(interactive, 'relay', return_value='AC'):
                result = interactive.execute(request, request['cases'][0], root / 'checker', lambda _: None)
        return result, commands

    def test_submission_memory_limit_is_separate_from_interactor_limit(self):
        from test_checker import job
        for memory in (64, 315, 512):
            with self.subTest(memory=memory):
                request = job()
                request['memoryLimitMb'] = memory
                request['interactor'] = request.pop('checker')
                result, commands = self.execute_stubbed(request, return_value=['/box/main'])
                self.assertEqual(result['verdict'], 'AC')
                self.assertIn(f'--cg-mem={memory * 1024}', commands[0])
                self.assertIn('--cg-mem=262144', commands[1])

    def test_interactor_heap_caps_stay_fixed_while_the_submission_scales(self):
        from test_checker import job
        v8 = '--max-old-space-size='
        expected = {'java25-isolate': ('-Xmx', {64: '32m', 256: '128m', 512: '256m'}, '128m'),
                    'csharp14-isolate': ('--env=DOTNET_GCHeapHardLimit=', {64: '0x1800000', 256: '0x6000000', 512: '0xc000000'}, '0x8000000'),
                    'go127-isolate': ('--env=GOMEMLIMIT=', {64: '48MiB', 256: '192MiB', 512: '384MiB'}, '160MiB'),
                    'ruby-truffle40-isolate': ('--env=RUBYOPT=', {64: '--vm.Xmx32m', 256: '--vm.Xmx128m', 512: '--vm.Xmx256m'}, '--vm.Xmx96m'),
                    'javascript-node24-isolate': ('--env=NODE_OPTIONS=', {64: v8 + '40', 256: v8 + '160', 512: v8 + '320'}, v8 + '128'),
                    'typescript-deno29-isolate': ('--env=DENO_V8_FLAGS=', {64: v8 + '40', 256: v8 + '160', 512: v8 + '320'}, v8 + '128')}
        for name, (prefix, submission, interactor) in expected.items():
            for memory, value in submission.items():
                with self.subTest(runtime=name, memory=memory):
                    request = job()
                    del request['checker']
                    request.update(runtime=name, memoryLimitMb=memory,
                                   interactor=dict(runtime=name.removesuffix('-isolate'), source='x'))
                    _, commands = self.execute_stubbed(
                        request, side_effect=lambda box, runtime, *args, **kwargs: list(runtime['run']))
                    self.assertEqual([[arg for arg in command if arg.startswith(prefix)] for command in commands],
                                     [[prefix + value], [prefix + interactor]])

    def test_java_interactor_heap_comes_from_the_flag_not_the_memory_value(self):
        # Like run_args, java_heap tells the interactor apart by its flag, so its cap ignores the memory value.
        command = list(interactive.RUNTIMES['java25-isolate']['run'])
        for memory in (64, 256, 512):
            with self.subTest(memory=memory):
                self.assertEqual([arg for arg in interactive.sandbox.java_heap(command, memory, interactor=True)
                                  if arg.startswith('-Xmx')], ['-Xmx128m'])

    def run_pair(self, source, interactor, wall=3, statuses=None, protocol='legacy', points=False):
        processes = [subprocess.Popen([sys.executable, '-c', code], stdin=subprocess.PIPE,
                                      stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, bufsize=0)
                     for code in (source, interactor)]
        log = []
        def observe(i):
            code = processes[i].poll()
            if code is None:
                return None
            return dict(status='' if code == 0 else 'RE', exitCode=max(0, code),
                        signal=max(0, -code), overflow=False, oom=False) | (statuses or {}).get(i, {})
        try:
            return interactive.relay(processes, observe, wall, log.append, protocol, points), ''.join(log)
        finally:
            for p in processes:
                if p.poll() is None:
                    p.kill()
                p.wait()
                p.stdin.close()
                p.stdout.close()

    def test_bidirectional_binary_data_and_eof(self):
        result, log = self.run_pair(
            "import sys; x=sys.stdin.buffer.read(4); assert x==b'a\\x00\\r\\n'; sys.stdout.buffer.write(x); sys.stdout.flush(); assert sys.stdin.buffer.read()==b''",
            "import sys; sys.stdout.buffer.write(b'a\\x00\\r\\n'); sys.stdout.flush(); assert sys.stdin.buffer.read(4)==b'a\\x00\\r\\n'")
        self.assertEqual(result, 'AC')
        self.assertIn('提出 → ジャッジ', log)
        self.assertIn('ジャッジ → 提出', log)

    def test_backpressure_and_eof_after_draining_buffer(self):
        result, _ = self.run_pair(
            "import sys; sys.stdout.buffer.write(b'x' * 1048576); sys.stdout.flush(); assert sys.stdin.read()=='ok'",
            "import sys,time; time.sleep(.05); x=b''\nwhile len(x)<1048576: x+=sys.stdin.buffer.read(min(4096,1048576-len(x)))\nassert x==b'x'*1048576; print('ok',end='')")
        self.assertEqual(result, 'AC')

    def test_rejection_does_not_wait_for_or_blame_peer(self):
        start = time.monotonic()
        result, _ = self.run_pair('import time; time.sleep(10)', 'assert False')
        self.assertEqual(result, 'WA')
        self.assertLess(time.monotonic()-start, 1)
        result, _ = self.run_pair('raise Exception()', 'import time; time.sleep(10)')
        self.assertEqual(result, 'RE')

    def test_testlib_failure_is_je_without_blaming_the_stopped_peer(self):
        for code, expected in [(1, 'WA'), (2, 'WA'), (3, 'JE'), (7, 'JE'), (8, 'WA')]:
            result, _ = self.run_pair('import time; time.sleep(10)', f'raise SystemExit({code})', protocol='testlib')
            self.assertEqual(result, expected)
        result, _ = self.run_pair('import time; time.sleep(10)', 'import os,signal; os.kill(os.getpid(),signal.SIGABRT)', protocol='testlib')
        self.assertEqual(result, 'JE')

    def test_testlib_points_wait_for_the_submission_in_aggregate_mode(self):
        result, _ = self.run_pair('pass', 'import time; time.sleep(.1); raise SystemExit(7)', protocol='testlib', points=True)
        self.assertEqual(result, 'AC')
        result, _ = self.run_pair('import time; time.sleep(10)', 'raise SystemExit(7)', wall=.3, protocol='testlib', points=True)
        self.assertEqual(result, 'TLE')
        result, _ = self.run_pair('raise SystemExit(2)', 'raise SystemExit(7)', protocol='testlib', points=True)
        self.assertEqual(result, 'RE')
        for code, expected in [(1, 'WA'), (3, 'JE')]:
            result, _ = self.run_pair('import time; time.sleep(10)', f'raise SystemExit({code})', protocol='testlib', points=True)
            self.assertEqual(result, expected)
        # Legacy interactors have no points exit code.
        result, _ = self.run_pair('import time; time.sleep(10)', 'raise SystemExit(7)', points=True)
        self.assertEqual(result, 'WA')

    def test_acceptance_requires_submission_exit_and_waits_have_shared_deadline(self):
        for interactor in ('pass', 'input()'):
            result, log = self.run_pair('import time; time.sleep(10)', interactor, wall=.15)
            self.assertEqual(result, 'TLE')
            self.assertIn('経過時間超過', log)

    def test_exit_between_observation_and_poll_cannot_skip_final_metrics(self):
        for source_code, judge_code, expected in [(2, 7, 'RE'), (0, 1, 'WA'), (0, 3, 'JE'), (0, 7, 'AC'), (0, 0, 'AC')]:
            with self.subTest(source=source_code, judge=judge_code):
                processes = [Mock(poll=Mock(return_value=code),
                                  stdin=Mock(closed=True), stdout=Mock(closed=True))
                             for code in (source_code, judge_code)]
                # Pipes have reached EOF, but both processes exit just after observe returns None.
                metrics = [dict(status='' if code == 0 else 'RE', exitCode=code,
                                signal=0, overflow=False, oom=False) for code in (source_code, judge_code)]
                observe = Mock(side_effect=[None, None, *metrics])
                with patch.object(interactive.os, 'set_blocking'), \
                        patch.object(interactive.select, 'select', return_value=([], [], [])):
                    result = interactive.relay(processes, observe, 3, lambda _: None, 'testlib', points=True)
                self.assertEqual(result, expected)
                self.assertEqual(observe.call_count, 4)

    def test_each_direction_and_stderr_resource_failure(self):
        with patch.object(interactive.sandbox, 'OUTPUT_LIMIT', 10000):
            for i in (0, 1):
                codes = ['import time; time.sleep(10)'] * 2
                codes[i] = "import sys; sys.stdout.write('x'*20000); sys.stdout.flush(); import time; time.sleep(10)"
                result, _ = self.run_pair(*codes)
                self.assertEqual(result, 'OLE' if i == 0 else 'JE')
        for i in (0, 1):
            codes = ['import time; time.sleep(10)'] * 2
            codes[i] = 'pass'
            result, _ = self.run_pair(*codes, statuses={i: {'overflow': True}})
            self.assertEqual(result, 'OLE' if i == 0 else 'JE')

    def test_resource_flags_of_each_side_follow_relay_rules(self):
        # The peer keeps running, so only the finished side's metrics decide the result.
        for i, flags, expected in [
                (0, {'oom': True}, 'MLE'), (0, {'status': 'TO'}, 'TLE'), (0, {'status': 'TO', 'oom': True}, 'MLE'),
                (0, {'overflow': True, 'oom': True}, 'OLE'), (0, {'exitCode': 1}, 'RE'),
                (1, {'oom': True}, 'JE'), (1, {'status': 'TO'}, 'JE'), (1, {'status': 'TO', 'wallTimeout': True}, 'TLE')]:
            with self.subTest(side=i, flags=flags):
                codes = ['import time; time.sleep(10)'] * 2
                codes[i] = 'pass'
                result, log = self.run_pair(*codes, statuses={i: flags})
                self.assertEqual(result, expected)
                if 'wallTimeout' in flags:
                    self.assertIn('経過時間超過', log)

    def test_total_output_limit_wins_over_finished_metrics(self):
        # The writer's metrics arrive in the same poll that first sees its total over the limit.
        for i, finished, expected in [
                (0, dict(status='', exitCode=0, signal=0, overflow=False, oom=True), 'OLE'),
                (0, dict(status='RE', exitCode=1, signal=0, overflow=False, oom=False), 'OLE'),
                (1, dict(status='RE', exitCode=1, signal=0, overflow=False, oom=False), 'JE')]:
            with self.subTest(side=i, finished=finished):
                codes = ['import time; time.sleep(10)'] * 2
                codes[i] = "import sys; sys.stdout.write('x'*20000); sys.stdout.flush(); import time; time.sleep(10)"
                processes = [subprocess.Popen([sys.executable, '-c', code], stdin=subprocess.PIPE,
                                              stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, bufsize=0)
                             for code in codes]
                log = []
                def observe(j):
                    # Only the writer sends 'x', so this counts the bytes relayed from it.
                    relayed = sum(entry.count('x') for entry in log)
                    return finished if j == i and relayed > 10000 else None
                try:
                    with patch.object(interactive.sandbox, 'OUTPUT_LIMIT', 10000):
                        result = interactive.relay(processes, observe, 3, log.append)
                finally:
                    for p in processes:
                        if p.poll() is None:
                            p.kill()
                        p.wait()
                        p.stdin.close()
                        p.stdout.close()
                self.assertEqual(result, expected)

    def test_jobs_reject_conflicting_modes_and_bad_interactors(self):
        from test_checker import job
        request = job()
        request['interactor'] = request['checker']
        with self.assertRaises(ValueError):
            host.validate_job(request, 'sha256:test')
        del request['checker']
        for code in ({}, False, {'runtime': 'sh', 'source': 'x'}, {'runtime': 'python314', 'source': ''}):
            request['interactor'] = code
            with self.assertRaises(ValueError):
                host.validate_job(request, 'sha256:test')
        request['interactor'] = {'runtime': 'python314', 'source': 'input()'}
        for field in ('generate', 'validate'):
            request[field] = True
            with self.assertRaises(ValueError):
                host.validate_job(request, 'sha256:test')
            del request[field]
        host.validate_job(request, 'sha256:test')

    def test_host_pins_artifacts_private_data_metrics_and_bounded_diagnostics(self):
        from test_checker import job
        request = job()
        request['interactor'] = request.pop('checker')
        request['easyTest'] = True
        compiled = []
        def compile(request, compile_phase=False, **kwargs):
            self.assertTrue(compile_phase)
            compiled.append(request['runtime'])
            Path(kwargs.get('artifact', host.sandbox.ARTIFACT)).write_text(request['source'])
            return dict(compiled=True)
        def execute(job, case, artifact, diagnostic, **kwargs):
            self.assertEqual(host.sandbox.ARTIFACT.read_text(), 'submitted source')
            self.assertEqual(artifact.read_text(), 'assert True')
            self.assertEqual(case['output'], 'expected secret\n')
            diagnostic('あ' * 20000)
            return dict(verdict='AC', cpuTimeMs=7, wallTimeMs=20, memoryBytes=512)
        with tempfile.TemporaryDirectory() as tmp, patch.object(host.sandbox, 'ARTIFACT', Path(tmp)/'main'), \
                patch.object(host.sandbox, 'META', Path(tmp)/'meta'), \
                patch.object(host.sandbox, 'execute', compile), patch.object(interactive, 'execute', execute):
            result = host.judge(request, 'sha256:test')
            self.assertEqual(list(Path(tmp).iterdir()), [])
        self.assertEqual(compiled, ['cpp23-gcc-isolate', 'python314-isolate'])
        self.assertEqual(result['passed'], 2)
        for case in result['cases']:
            self.assertNotIn('sampleDetails', case)
            self.assertNotIn('sampleOutput', case)
            self.assertTrue(case['checkerLog']['truncated'])
            self.assertLessEqual(len(case['checkerLog']['text'].encode()), 4096)
            self.assertTrue(case['checkerLog']['text'].startswith('あ'))
        self.assertEqual(result['cases'][0]['cpuTimeMs'], 7)
        self.assertLessEqual(len(result['checkerLog'].encode()), 16384)
