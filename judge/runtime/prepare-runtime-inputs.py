#!/usr/bin/env python3
"""Download upstream inputs once; subsequent builds verify the committed lock."""
import argparse
import concurrent.futures
import hashlib
import json
from pathlib import Path
import subprocess
import urllib.request

HERE = Path(__file__).resolve().parent
LOCK = HERE / 'runtime-sources.lock.json'
DEST = HERE.parent / '.build/runtime-inputs'


def fetch_json(url):
    with urllib.request.urlopen(urllib.request.Request(url, headers={'User-Agent': 'OpenOJ-runtime-build'}), timeout=60) as response:
        return json.load(response)


def sources():
    urls = {
        'gcc.tar.xz': 'https://gcc.gnu.org/pub/gcc/releases/gcc-16.2.0/gcc-16.2.0.tar.xz',
        'llvm.tar.xz': 'https://github.com/llvm/llvm-project/releases/download/llvmorg-23.1.1/LLVM-23.1.1-Linux-X64.tar.xz',
        'python.tar.xz': 'https://www.python.org/ftp/python/3.14.7/Python-3.14.7.tar.xz',
        'pypy.tar.bz2': 'https://downloads.python.org/pypy/pypy3.11-v7.3.23-linux64.tar.bz2',
        'codon.tar.gz': 'https://github.com/exaloop/codon/releases/download/v0.20.0/codon-linux-x86_64.tar.gz',
        'rust.tar.xz': 'https://static.rust-lang.org/dist/rust-1.98.1-x86_64-unknown-linux-gnu.tar.xz',
        'boost.tar.bz2': 'https://archives.boost.io/release/1.92.0/source/boost_1_92_0.tar.bz2',
        'acl.tar.gz': 'https://codeload.github.com/atcoder/ac-library/tar.gz/refs/tags/v1.6',
        'acl-python.tar.gz': 'https://codeload.github.com/not522/ac-library-python/tar.gz/27fdbb71cd0d566bdeb12746db59c9d908c6b5d5',
        'ac_library.jar': 'https://github.com/ocha98/ac-library-java/releases/download/v2.0.0/ac_library23.jar',
    }
    packages = {}
    for name in ('numpy', 'scipy', 'more-itertools', 'sortedcontainers'):
        packages[name] = fetch_json('https://pypi.org/pypi/' + name + '/json')['info']['version']
    files = {name: {'url': url} for name, url in urls.items()}
    files['llvm.tar.xz']['sha256'] = '832aeb58d105de1cabc7b982dd2c65de0610f7377df48ae8fc2dd8e97420a15c'
    files['codon.tar.gz']['sha256'] = '3699b803e65e3faf3321f114ae111a8981e11ac603a0d2b8447877063496c23b'
    return {'files': files, 'python_packages': packages}


def download(item):
    name, entry = item
    target = DEST / name
    if not target.exists():
        print('Downloading', name, flush=True)
        temporary = target.with_suffix(target.suffix + '.part')
        subprocess.run(['curl', '--fail', '--location', '--retry', '3', '--output', str(temporary), entry['url']], check=True)
        temporary.replace(target)
    with target.open('rb') as file:
        digest = hashlib.file_digest(file, 'sha256').hexdigest()
    if entry.get('sha256', digest) != digest:
        raise ValueError('checksum mismatch: ' + name)
    return name, dict(entry, sha256=digest)


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--lock', action='store_true', help='Resolve and pin upstream inputs for a new runtime')
    parser.add_argument('--extensions', action='store_true', help='Verify and download ADR 0010 inputs')
    parser.add_argument('--additions', action='store_true', help='Verify and download Haskell, JS/TS and Ruby inputs')
    args = parser.parse_args()
    if args.extensions and args.additions:
        parser.error('select one extension set')
    if args.extensions or args.additions:
        if args.lock:
            parser.error('extension versions must be selected and pinned explicitly')
        LOCK = HERE / ('runtime-addition-sources.lock.json' if args.additions else 'runtime-extension-sources.lock.json')
        DEST = HERE.parent / ('.build/addition-inputs' if args.additions else '.build/extension-inputs')
    manifest = sources() if args.lock else json.loads(LOCK.read_text())
    if not args.lock and any(len(entry.get('sha256', '')) != 64 for entry in manifest['files'].values()):
        parser.error('every locked input must have a SHA-256 checksum')
    DEST.mkdir(parents=True, exist_ok=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as executor:
        manifest['files'] = dict(executor.map(download, manifest['files'].items()))
    if args.lock:
        LOCK.write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
    (DEST / 'sources.json').write_text(json.dumps(manifest, indent=2, sort_keys=True) + '\n')
    if 'python_packages' in manifest:
        (DEST / 'requirements.txt').write_text(''.join(f'{name}=={version}\n' for name, version in manifest['python_packages'].items()))
    print('All runtime inputs verified.', flush=True)
