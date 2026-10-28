"""Tests for the pinned Bun release shared by the dev shell, CI, and the Lambda package."""
import hashlib
import io
import json
import shutil
import tempfile
import unittest
import zipfile
from pathlib import Path
from unittest import mock

import bun_release

WEB = Path(__file__).resolve().parents[1]


class CheckPinsTest(unittest.TestCase):
    def copy_repository(self):
        root = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, root)
        (root / 'web').mkdir()
        shutil.copy(WEB / 'package.json', root / 'web/package.json')
        shutil.copy(WEB.parent / 'flake.nix', root / 'flake.nix')
        return root

    def test_repository_pins_one_version(self):
        bun_release.check_pins(WEB)

    def test_rejects_another_package_manager_version(self):
        root = self.copy_repository()
        package = json.loads((root / 'web/package.json').read_text())
        package['packageManager'] = 'bun@1.0.0'
        (root / 'web/package.json').write_text(json.dumps(package))
        with self.assertRaisesRegex(SystemExit, 'packageManager'):
            bun_release.check_pins(root / 'web')

    def test_rejects_a_flake_hash_for_another_archive(self):
        root = self.copy_repository()
        flake = (root / 'flake.nix').read_text()
        (root / 'flake.nix').write_text(flake.replace('sha256-uqa7', 'sha256-AAAA'))
        with self.assertRaisesRegex(SystemExit, 'bun-linux-aarch64.zip'):
            bun_release.check_pins(root / 'web')


class BinaryTest(unittest.TestCase):
    def archive(self, cache, data):
        cache.mkdir(parents=True, exist_ok=True)
        buffer = io.BytesIO()
        with zipfile.ZipFile(buffer, 'w') as bundle:
            bundle.writestr('bun-linux-aarch64/bun', data)
        path = cache / f'bun-linux-aarch64-{bun_release.VERSION}.zip'
        path.write_bytes(buffer.getvalue())
        return hashlib.sha256(buffer.getvalue()).hexdigest()

    def test_extracts_a_verified_archive_without_downloading(self):
        cache = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, cache)
        digest = self.archive(cache, b'executable')
        with mock.patch.dict(bun_release.SHA256, {'bun-linux-aarch64.zip': digest}), \
                mock.patch('urllib.request.urlopen', side_effect=AssertionError('downloaded')):
            self.assertEqual(bun_release.binary('bun-linux-aarch64.zip', cache), b'executable')

    def test_rejects_an_archive_with_another_checksum(self):
        cache = Path(tempfile.mkdtemp())
        self.addCleanup(shutil.rmtree, cache)
        self.archive(cache, b'tampered')
        with self.assertRaisesRegex(SystemExit, 'SHA-256'):
            bun_release.binary('bun-linux-aarch64.zip', cache)


class HostAssetTest(unittest.TestCase):
    def cpuinfo(self, text):
        path = Path(tempfile.mkdtemp()) / 'cpuinfo'
        self.addCleanup(shutil.rmtree, path.parent)
        path.write_text(text)
        return path

    def test_picks_the_build_for_the_cpu(self):
        self.assertEqual(bun_release.host_asset('aarch64'), 'bun-linux-aarch64.zip')
        self.assertEqual(bun_release.host_asset('arm64'), 'bun-linux-aarch64.zip')
        self.assertEqual(bun_release.host_asset('x86_64', self.cpuinfo('flags : sse4_2 avx avx2')), 'bun-linux-x64.zip')
        self.assertEqual(bun_release.host_asset('x86_64', self.cpuinfo('flags : sse4_2 avx')), 'bun-linux-x64-baseline.zip')

    def test_falls_back_to_the_baseline_build_without_proc(self):
        # macOS has no /proc; the baseline build runs on every x86-64 CPU.
        self.assertEqual(bun_release.host_asset('x86_64', Path(tempfile.gettempdir()) / 'no-such-cpuinfo'), 'bun-linux-x64-baseline.zip')

    def test_rejects_another_machine(self):
        with self.assertRaisesRegex(SystemExit, 'riscv64'):
            bun_release.host_asset('riscv64')


if __name__ == '__main__':
    unittest.main()
