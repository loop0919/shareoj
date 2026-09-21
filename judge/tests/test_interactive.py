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
    def test_submission_memory_limit_is_separate_from_interactor_limit(self):
        from test_checker import job
        for memory in (64, 315, 512):
            with self.subTest(memory=memory), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                for i in range(2):
                    (root / str(i) / 'box').mkdir(parents=True)
                request = job()
                request['memoryLimitMb'] = memory
                request['interactor'] = request.pop('checker')
                commands = []
                def invoke(args, *, box_id=0, **kwargs):
                    return subprocess.CompletedProcess(args, 0, stdout=str(root / str(box_id)).encode())
                def popen(command, **kwargs):
                    commands.append(command)
                    meta = Path(next(arg.removeprefix('--meta=') for arg in command if arg.startswith('--meta=')))
                    meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                    return Mock(returncode=0, poll=Mock(return_value=0))
                with patch.object(interactive.sandbox, 'invoke', invoke), \
                        patch.object(interactive.sandbox, 'META', root / 'meta'), \
                        patch.object(interactive.sandbox, 'prepare_program', return_value=['/box/main']), \
                        patch.object(interactive.subprocess, 'Popen', popen), \
                        patch.object(interactive, 'relay', return_value='AC'):
                    result = interactive.execute(request, request['cases'][0], root / 'checker', lambda _: None)
                self.assertEqual(result['verdict'], 'AC')
                self.assertIn(f'--cg-mem={memory * 1024}', commands[0])
                self.assertIn('--cg-mem=262144', commands[1])

    def run_pair(self, source, interactor, wall=3, statuses=None, protocol='legacy'):
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
            return interactive.relay(processes, observe, wall, log.append, protocol), ''.join(log)
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

    def test_acceptance_requires_submission_exit_and_waits_have_shared_deadline(self):
        for interactor in ('pass', 'input()'):
            result, log = self.run_pair('import time; time.sleep(10)', interactor, wall=.15)
            self.assertEqual(result, 'TLE')
            self.assertIn('経過時間超過', log)

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
        def execute(job, case, artifact, diagnostic):
            self.assertEqual(host.sandbox.ARTIFACT.read_text(), 'submitted source')
            self.assertEqual(artifact.read_text(), 'assert True')
            self.assertEqual(case['output'], 'expected secret')
            diagnostic('あ' * 20000)
            return dict(verdict='AC', cpuTimeMs=7, wallTimeMs=20, memoryBytes=512)
        with tempfile.TemporaryDirectory() as tmp, patch.object(host.sandbox, 'ARTIFACT', Path(tmp)/'main'), \
                patch.object(host.sandbox, 'META', Path(tmp)/'meta'), \
                patch.object(host.sandbox, 'execute', compile), patch.object(interactive, 'execute', execute):
            result = host.judge(request, 'sha256:test')
            self.assertEqual(list(Path(tmp).iterdir()), [])
        self.assertEqual(compiled, ['cpp17-isolate', 'python314-isolate'])
        self.assertEqual(result['passed'], 2)
        for case in result['cases']:
            self.assertNotIn('sampleDetails', case)
            self.assertNotIn('sampleOutput', case)
            self.assertTrue(case['checkerLog']['truncated'])
            self.assertLessEqual(len(case['checkerLog']['text'].encode()), 4096)
            self.assertTrue(case['checkerLog']['text'].startswith('あ'))
        self.assertEqual(result['cases'][0]['cpuTimeMs'], 7)
        self.assertLessEqual(len(result['checkerLog'].encode()), 16384)
