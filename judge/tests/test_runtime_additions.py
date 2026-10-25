import sys
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'runner'))
import sandbox
from runtimes import RUNTIMES
from interactive_smoke import failure_program


class RuntimeAdditionTests(unittest.TestCase):
    def test_caught_file_limit_signal_cannot_hide_output_overflow(self):
        args = sandbox.run_args('javascript-node24-isolate', 1, 4, 512)
        file_limit = int(next(a.split('=')[1] for a in args if a.startswith('--fsize='))) * 1024
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / 'stdout'
            subprocess.run([sys.executable, '-c', '''
import errno, resource, signal, sys
resource.setrlimit(resource.RLIMIT_FSIZE, (int(sys.argv[2]), int(sys.argv[2])))
signal.signal(signal.SIGXFSZ, signal.SIG_IGN)
with open(sys.argv[1], 'wb', buffering=0) as file:
    try:
        while True: file.write(b'x' * 65536)
    except OSError as error:
        assert error.errno == errno.EFBIG
''', str(output), str(file_limit)], check=True)
            self.assertEqual(len(sandbox.regular_read(output, sandbox.OUTPUT_LIMIT)), sandbox.OUTPUT_LIMIT + 1)
            with output.open('r+b') as file:
                file.truncate(sandbox.OUTPUT_LIMIT)
            self.assertEqual(len(sandbox.regular_read(output, sandbox.OUTPUT_LIMIT)), sandbox.OUTPUT_LIMIT)

    def test_compiled_typescript_is_collected_and_keeps_javascript_suffix(self):
        runtime = RUNTIMES['typescript-node24-isolate']
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'compiled').mkdir()
            (root / 'compiled/main.js').write_bytes(b'console.log(3)')
            data = sandbox.collect_artifact(root, runtime)
            artifact = root / 'artifact'
            artifact.write_bytes(data)
            box = root / 'fresh'
            box.mkdir()
            command = sandbox.prepare_program(box, runtime, artifact)
            self.assertEqual(command[-1], '/box/main.js')
            self.assertEqual((box / 'main.js').read_bytes(), data)
            self.assertFalse((box / 'main.ts').exists())

    def test_compiler_output_rejects_symlink_parent_and_file(self):
        runtime = RUNTIMES['typescript-bun14-isolate']
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'real').mkdir()
            (root / 'real/main.js').write_text('secret')
            (root / 'compiled').symlink_to(root / 'real', target_is_directory=True)
            with self.assertRaises(ValueError):
                sandbox.collect_artifact(root, runtime)
            (root / 'compiled').unlink()
            (root / 'compiled').mkdir()
            (root / 'compiled/main.js').symlink_to(root / 'real/main.js')
            with self.assertRaises(OSError):
                sandbox.collect_artifact(root, runtime)

    def test_deno_cache_is_private_and_dependencies_are_mounted_read_only(self):
        runtime = RUNTIMES['typescript-deno29-isolate']
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            cache = root / 'deno-deps/cache'
            cache.mkdir(parents=True)
            (cache / 'entry').write_text('original')
            with patch.object(sandbox, 'ROOT', str(root)):
                for name in ('first', 'second'):
                    box = root / name
                    box.mkdir()
                    sandbox.prepare_dependencies(box, runtime)
                (root / 'first/deno-cache/entry').write_text('changed')
                self.assertEqual((root / 'second/deno-cache/entry').read_text(), 'original')
                self.assertEqual((cache / 'entry').read_text(), 'original')
                args = sandbox.run_args('typescript-deno29-isolate', 1, 4, 512)
                mount = next(arg for arg in args if arg.startswith('--dir=/box/node_modules='))
                self.assertNotIn(':rw', mount)
                self.assertFalse(any(arg.startswith('--syscalls=') for arg in args))

    def test_every_registered_runtime_has_an_explicit_checker_failure(self):
        for name in RUNTIMES:
            if name == 'text-isolate':
                continue  # Text is not a judge-code language.
            self.assertTrue(failure_program(name))
        with self.assertRaises(KeyError):
            failure_program('unknown-isolate')
