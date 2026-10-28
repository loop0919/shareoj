"""The Bun release that the dev shell, CI, and the Lambda package share.

Usage: python3 scripts/bun_release.py install <directory>
Installs bun and bunx for this machine where oven-sh/setup-bun cannot run, such as the
Playwright image, which has no unzip.
"""
import base64
import hashlib
import io
import json
import platform
import re
import sys
import tempfile
import urllib.request
import zipfile
from pathlib import Path

VERSION = '1.4.4'
# From the release's SHASUMS256.txt. Update with flake.nix and web/package.json.
SHA256 = {
    'bun-linux-aarch64.zip': 'baa6bb36f51d6e58561e02bbea825038a9df454f673d6d776e8bbf86f8c7850e',
    'bun-linux-x64.zip': 'bf8ccbfb2b0b80159e0f9a6d15c55317add8077534dbc6130a3edb52fccb395c',
    'bun-linux-x64-baseline.zip': 'e0446eb55995772eb9894bebae9a6be368d985a8b9271ee124486f9946eedd76',
}
WEB = Path(__file__).resolve().parents[1]


def check_pins(web=WEB):
    """Fail unless package.json and the dev shell name this release."""
    manager = json.loads((web / 'package.json').read_text()).get('packageManager')
    if manager != f'bun@{VERSION}':
        raise SystemExit(f'web/package.json has packageManager {manager}; expected bun@{VERSION}')
    flake = (web.parent / 'flake.nix').read_text()
    version = re.search(r'pkgs\.bun\.overrideAttrs.*?version = "([^"]+)"', flake, re.S)
    if not version or version[1] != VERSION:
        raise SystemExit(f'flake.nix pins Bun {version and version[1]}; expected {VERSION}')
    for asset in ('bun-linux-aarch64.zip', 'bun-linux-x64-baseline.zip'):
        pinned = re.search(rf'asset = "{re.escape(asset)}";\s*hash = "([^"]+)"', flake)
        expected = 'sha256-' + base64.b64encode(bytes.fromhex(SHA256[asset])).decode()
        if not pinned or pinned[1] != expected:
            raise SystemExit(f'flake.nix hash for {asset} differs from scripts/bun_release.py')


def binary(asset, cache):
    """The bun executable from the release ZIP, downloaded once into cache and verified."""
    name = asset.removesuffix('.zip')
    archive = cache / f'{name}-{VERSION}.zip'
    if not archive.exists():
        cache.mkdir(parents=True, exist_ok=True)
        url = f'https://github.com/oven-sh/bun/releases/download/bun-v{VERSION}/{asset}'
        with urllib.request.urlopen(url, timeout=120) as response:
            partial = archive.with_suffix('.partial')
            partial.write_bytes(response.read())
        partial.replace(archive)
    data = archive.read_bytes()
    if hashlib.sha256(data).hexdigest() != SHA256[asset]:
        raise SystemExit(f'{archive} does not match the pinned SHA-256; delete it and retry')
    return zipfile.ZipFile(io.BytesIO(data)).read(f'{name}/bun')


def host_asset():
    machine = platform.machine()
    if machine in ('aarch64', 'arm64'):
        return 'bun-linux-aarch64.zip'
    if machine == 'x86_64':
        # The same check as Bun's installer: the default build needs AVX2.
        return 'bun-linux-x64.zip' if 'avx2' in Path('/proc/cpuinfo').read_text() else 'bun-linux-x64-baseline.zip'
    raise SystemExit(f'Unsupported machine: {machine}')


def install(directory):
    target = Path(directory)
    target.mkdir(parents=True, exist_ok=True)
    bun = target / 'bun'
    bun.write_bytes(binary(host_asset(), Path(tempfile.gettempdir()) / 'bun-release'))
    bun.chmod(0o755)
    bunx = target / 'bunx'
    bunx.unlink(missing_ok=True)
    bunx.symlink_to('bun')
    print(f'Installed Bun {VERSION} to {bun}')


if __name__ == '__main__':
    check_pins()
    if len(sys.argv) != 3 or sys.argv[1] != 'install':
        raise SystemExit(__doc__)
    install(sys.argv[2])
