import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[1]


class ReleaseTests(unittest.TestCase):
    def test_build_keeps_the_installer_layout_and_file_contents(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for directory in ('runner', 'deploy'):
                shutil.copytree(ROOT / directory, root / directory)
            shutil.copy2(ROOT / 'build-assets.sh', root)
            (root / '.build').mkdir()
            (root / '.build/runtime-additions.tar.gz').write_bytes(b'runtime fixture')
            bin_dir = root / 'bin'
            bin_dir.mkdir()
            curl = bin_dir / 'curl'
            curl.write_text('#!/bin/sh\nwhile [ "$1" != "-o" ]; do shift; done\nprintf isolate > "$2"\n')
            curl.chmod(0o755)
            env = dict(os.environ, PATH=str(bin_dir) + os.pathsep + os.environ['PATH'])
            env.pop('RUNTIME_ARCHIVE', None)
            env.pop('ISOLATE_COMMIT', None)
            subprocess.run(['bash', str(root / 'build-assets.sh')], cwd='/', env=env,
                           check=True, capture_output=True)
            expected = {
                name: (ROOT / 'runner' / name).read_bytes()
                for name in ('host.py', 'sandbox.py', 'interactive.py', 'interactive_smoke.py',
                             'testlib_smoke.py', 'worker.py', 'telemetry.py', 'pool.py', 'smoke.py',
                             'fingerprint.py', 'smoke.sh', 'runtimes.py', 'language-smoke.json')
            }
            expected.update({name: (ROOT / 'deploy' / name).read_bytes()
                             for name in ('install.sh', 'judge-worker.service')})
            expected.update({'runtime.tar.gz': b'runtime fixture', 'isolate.tar.gz': b'isolate',
                             'isolate-commit': b'8f185bb37f3f23e29b33b0c7727c91c13429abe3\n'})
            with tarfile.open(root / '.build/worker.tar.gz') as archive:
                actual = {member.name.removeprefix('./'): archive.extractfile(member).read()
                          for member in archive.getmembers() if member.isfile()}
            self.assertEqual(actual, expected)
