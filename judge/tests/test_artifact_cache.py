import io
import json
import logging
import os
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from test_runner import host, sandbox
from test_checker import job, reply
import telemetry


class ArtifactCacheTests(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        (self.root / 'run').mkdir()
        self.artifact = self.root / 'run' / 'checker'
        self.compiles = []
        self.compiled = True

        def execute(request, compile_phase=False, **kwargs):
            if compile_phase:
                self.compiles.append((request['runtime'], request['source']))
                if self.compiled:
                    Path(kwargs.get('artifact', sandbox.ARTIFACT)).write_bytes(b'built:' + request['source'].encode())
                return dict(compiled=self.compiled, compileLog='log')
            return reply()

        for patcher in (patch.object(sandbox, 'execute', execute),
                        patch.object(sandbox, 'ARTIFACT', self.root / 'run' / 'main'),
                        patch.object(sandbox, 'META', self.root / 'run' / 'meta'),
                        patch.dict(os.environ, {'CACHE_DIRECTORY': str(self.root / 'cache')})):
            patcher.start()
            self.addCleanup(patcher.stop)

    def compile(self, source='code', runtime='cpp23-gcc-isolate', digest='sha256:a'):
        self.artifact.unlink(missing_ok=True)
        return sandbox.execute_cached(dict(source=source, runtime=runtime), digest, self.artifact)

    def test_same_code_under_same_digest_compiles_once(self):
        first = self.compile()
        second = self.compile()
        self.assertEqual(len(self.compiles), 1)
        self.assertTrue(first['compiled'] and second['compiled'])
        self.assertEqual((first.get('cached', False), second['cached']), (False, True))
        self.assertEqual(self.artifact.read_bytes(), b'built:code')
        self.assertEqual(self.artifact.stat().st_mode & 0o777, 0o500)

    def test_another_source_runtime_or_digest_compiles_again(self):
        self.compile()
        self.compile(source='code2')
        self.compile(runtime='cpp23-clang-isolate')
        self.compile(digest='sha256:b')
        self.assertEqual(len(self.compiles), 4)
        self.assertEqual(self.artifact.read_bytes(), b'built:code')

    def test_failed_compile_is_not_kept(self):
        self.compiled = False
        self.assertFalse(self.compile()['compiled'])
        self.assertFalse(self.compile()['compiled'])
        self.assertEqual(len(self.compiles), 2)

    def test_without_cache_directory_every_compile_runs(self):
        with patch.dict(os.environ):
            del os.environ['CACHE_DIRECTORY']
            self.compile()
            self.compile()
        self.assertEqual(len(self.compiles), 2)
        self.assertFalse((self.root / 'cache').exists())

    def test_unusable_cache_directory_still_compiles(self):
        (self.root / 'cache').write_text('not a directory')
        self.assertTrue(self.compile()['compiled'])
        self.assertTrue(self.compile()['compiled'])
        self.assertEqual(len(self.compiles), 2)
        self.assertEqual(self.artifact.read_bytes(), b'built:code')

    def test_oldest_entries_leave_when_over_the_limit(self):
        # Each artifact is 7 bytes ('built:' + one letter); two fit, three do not.
        with patch.object(sandbox, 'CACHE_LIMIT', 15):
            for age, source in enumerate('abc'):
                self.compile(source=source)
                for entry in (self.root / 'cache' / 'artifacts').iterdir():
                    # Make ages distinct without sleeping: older entries get older stamps.
                    stamp = entry.stat().st_mtime - 10
                    os.utime(entry, (stamp, stamp))
            self.compiles.clear()
            self.compile(source='c')
            self.compile(source='b')
            self.assertEqual(self.compiles, [])
            self.compile(source='a')
            self.assertEqual(len(self.compiles), 1)

    def test_a_hit_counts_as_recent_use(self):
        with patch.object(sandbox, 'CACHE_LIMIT', 15):
            self.compile(source='a')
            self.compile(source='b')
            for entry in (self.root / 'cache' / 'artifacts').iterdir():
                os.utime(entry, (1000, 1000) if entry.read_bytes() == b'built:a' else (2000, 2000))
            self.compile(source='a')  # 'a' is now the most recent, so 'b' leaves next.
            self.compile(source='c')
            self.compiles.clear()
            self.compile(source='a')
            self.assertEqual(self.compiles, [])
            self.compile(source='b')
            self.assertEqual(len(self.compiles), 1)

    def test_judge_reuses_checker_and_records_compile_events(self):
        output = io.StringIO()
        handler = logging.StreamHandler(output)
        old = telemetry.logger.handlers[:], telemetry.logger.level
        telemetry.logger.handlers = [handler]
        telemetry.logger.setLevel(logging.INFO)
        self.addCleanup(lambda: (setattr(telemetry.logger, 'handlers', old[0]), telemetry.logger.setLevel(old[1])))
        for _ in range(2):
            self.assertEqual(host.judge(job(), 'sha256:test')['verdict'], 'AC')
        # The submission compiles every time; the checker only once.
        self.assertEqual(self.compiles, [('cpp23-gcc-isolate', 'submitted source'), ('python314-isolate', 'assert True'),
                                         ('cpp23-gcc-isolate', 'submitted source')])
        events = [json.loads(line) for line in output.getvalue().splitlines()]
        events = [e for e in events if e['event'] == 'judge_code_compile']
        self.assertEqual([(e['target'], e['cache']) for e in events], [('checker', 'miss'), ('checker', 'hit')])
        self.assertTrue(all(type(e['compileMs']) is int and e['compileMs'] >= 0 for e in events))
        self.assertEqual(sorted(p.name for p in (self.root / 'run').iterdir()), [])

    def test_worker_unit_provides_the_cache_directory(self):
        unit = (Path(__file__).resolve().parents[1] / 'deploy' / 'judge-worker.service').read_text()
        self.assertIn('\nCacheDirectory=judge\n', unit)
        self.assertIn('\nCacheDirectoryMode=0700\n', unit)


if __name__ == '__main__':
    unittest.main()
