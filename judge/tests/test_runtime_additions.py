import json
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

    def test_ruby_runs_do_not_load_bundler(self):
        for name in ('ruby40-isolate', 'ruby-truffle40-isolate'):
            runtime = RUNTIMES[name]
            self.assertFalse(any('bundler' in arg for arg in runtime['run']), name)
            self.assertFalse(any(env.startswith('BUNDLE_') for env in runtime['env']), name)
            gem_home = next(env for env in runtime['env'] if env.startswith('GEM_HOME='))
            self.assertIn('GEM_PATH=' + gem_home.split('=', 1)[1], runtime['env'])

    def test_goose_builds_with_the_judge_gcc_and_keeps_its_cache_in_the_box(self):
        runtime = RUNTIMES['goose20263-isolate']
        compile = runtime['compile']
        self.assertIn('--standalone', compile)
        self.assertEqual(compile[compile.index('--cc') + 1], '/opt/judge-runtimes/gcc/bin/gcc')
        self.assertEqual([env for env in runtime['env'] if env.startswith('GOOSE_HOME=')], ['GOOSE_HOME=/box/goose-home'])

    def test_sbcl_compiles_and_runs_with_the_preloaded_image(self):
        runtime = RUNTIMES['commonlisp-sbcl26-isolate']
        self.assertEqual(runtime['source'], 'main.lisp')
        self.assertEqual(runtime['compile'][0], '/opt/judge-runtimes/sbcl/bin/sbcl-judge')
        self.assertEqual(runtime['run'][0], '/opt/judge-runtimes/sbcl/bin/sbcl-judge')
        # No init files: a submission must not depend on the operator's home.
        self.assertFalse(any('sbclrc' in arg for arg in runtime['compile'] + runtime['run']))

    def test_sbcl_collects_garbage_before_its_heap_growth_reaches_the_memory_limit(self):
        def nursery(memory, **role):
            args = sandbox.run_args('commonlisp-sbcl26-isolate', 1, 4, memory, **role)
            return [int(arg.split('=')[2]) >> 20 for arg in args if arg.startswith('--env=SBCL_BYTES_CONSED_BETWEEN_GCS=')]
        self.assertEqual(nursery(512), [51])
        self.assertEqual(nursery(64), [6])
        self.assertEqual(nursery(1024, compile_phase=True), [51])
        self.assertEqual(nursery(256, interactor=True), [25])

    def test_fsharp_compiles_without_msbuild_and_ships_its_core_library(self):
        runtime = RUNTIMES['fsharp10-isolate']
        self.assertEqual(runtime['artifact'], 'dotnet')
        self.assertEqual(runtime['compile'][:2], ['/opt/judge-runtimes/dotnet/dotnet', '/opt/judge-runtimes/fsharp-tools/fsc.dll'])
        self.assertEqual(runtime['run'], RUNTIMES['csharp14-isolate']['run'])
        self.assertIn('FSharp.Core.dll', runtime['files'])
        self.assertEqual(RUNTIMES['csharp14-isolate']['files'],
                         ['main.runtimeconfig.json', 'main.deps.json', 'MathNet.Numerics.dll', 'ac-library-csharp.dll'])

    def test_fsharp_box_gets_one_dll_per_locked_package(self):
        # The build checks fsharp-libs against the same lock.
        lock = Path(__file__).resolve().parents[1] / 'runtime/deps/additions/fsharp/packages.lock.json'
        packages = json.loads(lock.read_text())['dependencies']['net10.0']
        self.assertEqual(sorted(RUNTIMES['fsharp10-isolate']['files']),
                         sorted(['main.runtimeconfig.json', *[name + '.dll' for name in packages]]))
        self.assertEqual(RUNTIMES['fsharp10-isolate']['files_root'], 'fsharp-libs')

    def test_fsharp_runs_in_the_same_sandbox_as_csharp_without_write_xor_execute(self):
        # Same heap caps per role and the compiler's 256 open files. W^X would cap fsc's code at the file size limit.
        wx = '--env=DOTNET_EnableWriteXorExecute=0'
        for memory, role in ((512, {}), (64, {}), (1024, dict(compile_phase=True)), (256, dict(interactor=True))):
            with self.subTest(memory=memory, **role):
                fsharp = sandbox.run_args('fsharp10-isolate', 1, 4, memory, **role)
                self.assertIn(wx, fsharp)
                self.assertEqual([arg for arg in fsharp if arg != wx], sandbox.run_args('csharp14-isolate', 1, 4, memory, **role))

    def test_gleam_runs_on_one_scheduler_without_the_gleam_cli(self):
        runtime = RUNTIMES['gleam119-isolate']
        self.assertEqual(runtime['compile'], ['/usr/bin/sh', '/opt/judge-runtimes/gleam/compile.sh'])
        run = runtime['run']
        self.assertEqual(run[0], '/opt/judge-runtimes/erlang/bin/erl')
        for flag, value in (('+S', '1:1'), ('+SDcpu', '1:1'), ('+SDio', '1'), ('+A', '1')):
            self.assertEqual(run[run.index(flag) + 1], value)
        self.assertIn('-noshell', run)
        self.assertFalse(any(arg.endswith('/gleam') for arg in run))

    def test_gleam_stays_within_the_file_size_limit_and_keeps_stdin(self):
        runtime = RUNTIMES['gleam119-isolate']
        # The double-mapped JIT memory is a memfd that the file size limit kills with SIGXFSZ.
        self.assertEqual(runtime['run'][runtime['run'].index('+JMsingle') + 1], 'true')
        # A crash dump in /box could turn an RE into OLE; -noinput would cut stdin off.
        self.assertIn('ERL_CRASH_DUMP_SECONDS=0', runtime['env'])
        self.assertNotIn('-noinput', runtime['run'])

    def test_gleam_judge_files_are_plain_arguments(self):
        runtime = RUNTIMES['gleam119-isolate']
        self.assertEqual(runtime['run'][-1], '-extra')
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            artifact = root / 'artifact'
            artifact.write_bytes(b'FOR1')
            for files, extra in ((None, []), (dict.fromkeys(('test-input', 'expected-output', 'submission-source'), b''),
                                              ['/box/test-input', '/box/expected-output', '/box/submission-source', '/box/score'])):
                box = root / ('judge' if files else 'submission')
                box.mkdir()
                command = sandbox.prepare_program(box, runtime, artifact, files)
                self.assertEqual(command[command.index('-extra') + 1:], extra)
                self.assertEqual((box / 'main.beam').read_bytes(), b'FOR1')

    def test_gleam_template_pins_every_listed_library(self):
        import tomllib
        deps = Path(__file__).resolve().parents[1] / 'runtime/deps/additions/gleam'
        listed = tomllib.loads((deps / 'gleam.toml').read_text())['dependencies']
        self.assertEqual(sorted(listed), ['bidict', 'bigdecimal', 'bitsandbobs', 'dijkstra', 'gleam_community_maths',
                                          'gleam_deque', 'gleam_regexp', 'gleam_stdlib', 'gleam_yielder', 'gleamy_structures',
                                          'glearray', 'ieee_float', 'iv', 'stdin', 'trie_again'])
        manifest = tomllib.loads((deps / 'manifest.toml').read_text())
        packages = {package['name']: package for package in manifest['packages']}
        self.assertLessEqual(set(listed), set(packages))
        for name, package in packages.items():
            self.assertEqual(package['source'], 'hex', name)
            self.assertRegex(package['outer_checksum'], '^[0-9A-F]{64}$', name)
        self.assertEqual({name: value['version'] for name, value in manifest['requirements'].items()}, listed)

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
