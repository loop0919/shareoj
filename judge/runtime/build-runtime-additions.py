#!/usr/bin/env python3
"""Build the additional runtimes in Ubuntu; RESOLVE=1 is only for updating locks."""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import zipfile

ROOT = Path('/opt/judge-runtimes')
INPUT = Path('/inputs/additions')
DEPS = Path('/build/deps')
WORK = Path('/build/additions')
RESOLVE = os.environ.get('RESOLVE') == '1'
WORK.mkdir(exist_ok=True)
CATALOG = json.loads((DEPS / 'catalog.json').read_text())['packages']


def run(*args, cwd=WORK, **kwargs):
    subprocess.run([str(a) for a in args], cwd=cwd, check=True, **kwargs)


def unpack(name, destination, strip=True):
    destination.mkdir(parents=True, exist_ok=True)
    run('tar', '-xf', INPUT / name, '-C', destination, *(['--strip-components=1'] if strip else []))


def copy_lock(name, destination):
    source = DEPS / name
    if source.exists():
        shutil.copyfile(source, destination)
    elif not RESOLVE:
        raise RuntimeError('Missing lock: ' + name + '; resolve and review it before a release build')


def save_lock(source, name=None):
    target = ROOT / 'build-manifest/additions'
    target.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(source, target / (name or source.name))


def toolchains():
    manifest = json.loads((INPUT / 'sources.json').read_text())
    with Path('/inputs/base.tar.gz').open('rb') as f:
        assert hashlib.file_digest(f, 'sha256').hexdigest() == manifest['base_archive_sha256']
    for name, item in manifest['files'].items():
        with (INPUT / name).open('rb') as f:
            assert hashlib.file_digest(f, 'sha256').hexdigest() == item['sha256'], name
    run('tar', '-xf', '/inputs/base.tar.gz', '-C', '/opt')
    if (ROOT / 'java').exists():
        shutil.rmtree(ROOT / 'java')
    (ROOT / 'java-libs/bifurcan.jar').unlink(missing_ok=True)
    unpack('node.tar.xz', ROOT / 'node')
    with zipfile.ZipFile(INPUT / 'bun.zip') as z:
        name = next(n for n in z.namelist() if n.endswith('/bun'))
        (ROOT / 'bun').mkdir()
        (ROOT / 'bun/bun').write_bytes(z.read(name))
        (ROOT / 'bun/bun').chmod(0o755)
    with zipfile.ZipFile(INPUT / 'deno.zip') as z:
        (ROOT / 'deno').mkdir()
        (ROOT / 'deno/deno').write_bytes(z.read('deno'))
        (ROOT / 'deno/deno').chmod(0o755)
    unpack('ghc.tar.xz', WORK / 'ghc')
    run('./configure', '--prefix=' + str(ROOT / 'ghc'), cwd=WORK / 'ghc')
    run('make', 'install', cwd=WORK / 'ghc')
    shutil.rmtree(ROOT / 'ghc/share/doc', ignore_errors=True)
    unpack('cabal.tar.xz', ROOT / 'cabal/bin', strip=False)
    unpack('ruby.tar.xz', WORK / 'ruby')
    run('./configure', '--prefix=' + str(ROOT / 'ruby'), '--disable-install-doc',
        '--enable-yjit', '--disable-zjit', '--enable-shared', 'RUSTC=' + str(ROOT / 'rust/bin/rustc'),
        'LDFLAGS=-Wl,-rpath,' + str(ROOT / 'ruby/lib'), cwd=WORK / 'ruby')
    run('make', '-j4', cwd=WORK / 'ruby')
    run('make', 'install', cwd=WORK / 'ruby')
    run(ROOT / 'ruby/bin/ruby', '--yjit', '-e', 'abort "YJIT is disabled" unless RubyVM::YJIT.enabled?')
    save_lock(INPUT / 'sources.json')
    save_lock(DEPS / 'catalog.json')


def javascript():
    target = ROOT / 'js-deps'
    target.mkdir(exist_ok=True)
    shutil.copyfile(DEPS / 'package.json', target / 'package.json')
    copy_lock('package-lock.json', target / 'package-lock.json')
    run('npm', 'install' if RESOLVE else 'ci', '--ignore-scripts', '--no-audit', '--no-fund', cwd=target)
    save_lock(target / 'package.json')
    save_lock(target / 'package-lock.json')
    deno = ROOT / 'deno-deps'
    deno.mkdir(exist_ok=True)
    (deno / 'node_modules').unlink(missing_ok=True)
    (deno / 'node_modules').symlink_to(target / 'node_modules', target_is_directory=True)
    shutil.copyfile(DEPS / 'deno.json', deno / 'deno.json')
    copy_lock('deno.lock', deno / 'deno.lock')
    # Cache every advertised dependency, not just those imported by one example.
    (deno / 'all.ts').write_text(''.join('import ' + json.dumps(name) + ';\n'
        for name in json.loads((DEPS / 'deno-cache-entries.json').read_text())))
    env = dict(os.environ, DENO_DIR=str(deno / 'cache'), DENO_NO_UPDATE_CHECK='1')
    run(ROOT / 'deno/deno', 'cache', *(['--frozen=false'] if RESOLVE else []),
        '--config', deno / 'deno.json', deno / 'all.ts', cwd=deno, env=env)
    save_lock(deno / 'deno.json')
    save_lock(deno / 'deno.lock')
    save_lock(DEPS / 'deno-std-versions.json')


def ruby_native():
    unpack('truffleruby.tar.gz', ROOT / 'truffleruby')
    native = ROOT / 'addition-native/lib'
    native.mkdir(parents=True, exist_ok=True)
    with zipfile.ZipFile(INPUT / 'z3.zip') as archive:
        for name in archive.namelist():
            if '/bin/libz3.so' in name and not name.endswith('/'):
                (native / Path(name).name).write_bytes(archive.read(name))
            elif Path(name).name in ('LICENSE.txt', 'LICENSE'):
                (native / 'Z3-LICENSE.txt').write_bytes(archive.read(name))
    # numo-linalg's extconf uses fiddle without declaring a build dependency.
    run(ROOT / 'ruby/bin/gem', 'install', '--local', INPUT / 'fiddle.gem', '--no-document')


def ruby(truffle=False):
    target = ROOT / ('truffleruby-deps' if truffle else 'ruby-deps')
    target.mkdir(exist_ok=True)
    gemfile = 'TruffleGemfile' if truffle else 'Gemfile'
    shutil.copyfile(DEPS / gemfile, target / 'Gemfile')
    copy_lock(gemfile + '.lock', target / 'Gemfile.lock')
    env = dict(os.environ, BUNDLE_GEMFILE=str(target / 'Gemfile'), BUNDLE_PATH=str(target / 'bundle'),
               BUNDLE_JOBS='1', BUNDLE_FROZEN='false' if RESOLVE else 'true')
    env['MAKEFLAGS'] = '-j1'
    env['TARGET'] = 'CORE2'
    env['NUM_THREADS'] = '1'
    if not truffle:
        # mkmf otherwise selects Ubuntu's header before the gem's bundled OpenBLAS header.
        env['BUNDLE_BUILD__NUMO___OPENBLAS'] = '--with-cppflags="-I' + str(
            target / 'bundle/ruby/4.0.0/gems/numo-openblas-0.5.5/vendor/include') + ' -DOPENBLAS_NUM_CORES=1"'
    if truffle:
        env['PATH'] = str(ROOT / 'truffleruby/bin') + ':' + env['PATH']
        env['RUBYOPT'] = '--vm.Xmx1g'
    run('bundle', 'install', cwd=target, env=env)
    run('bundle', 'clean', '--force', cwd=target, env=env)
    run('bundle', 'lock', '--add-checksums', cwd=target, env=env)
    run('bundle', 'cache', cwd=target, env=env)
    sources = json.loads((DEPS / 'native-sources.lock.json').read_text())
    if not truffle:
        archive = next(target.glob('bundle/ruby/*/gems/numo-openblas-*/vendor/tmp/openblas.tgz'))
        with archive.open('rb') as file:
            if hashlib.file_digest(file, 'sha256').hexdigest() != sources['numo-openblas']['sha256']:
                raise RuntimeError('OpenBLAS source checksum changed')
        vendor = next(target.glob('bundle/*/*/gems/or-tools-*/ext/or-tools/vendor.rb')).read_text()
        if sources['or-tools']['sha256'] not in vendor or sources['or-tools']['version'] not in vendor:
            raise RuntimeError('OR-Tools source lock changed')
    save_lock(DEPS / 'native-sources.lock.json')
    save_lock(target / 'Gemfile', gemfile)
    save_lock(target / 'Gemfile.lock', gemfile + '.lock')


def haskell():
    target = ROOT / 'haskell-deps'
    target.mkdir(exist_ok=True)
    project = WORK / 'haskell'
    project.mkdir(exist_ok=True)
    # Boot libraries must match GHC. Cabal resolves the remaining advertised set together.
    boot = subprocess.check_output(['ghc-pkg', 'list', '--simple-output'], text=True).split()
    boot_names = {re.sub(r'-[0-9].*', '', name) for name in boot}
    packages = [p for p in CATALOG['haskell'] if p not in boot_names and p != 'alex']
    (project / 'dependencies.cabal').write_text('cabal-version: 3.0\nname: judge-dependencies\nversion: 1.0.0\n'
        'executable dependencies\n  main-is: Main.hs\n  default-language: Haskell2010\n  build-depends: '
        + ', '.join(['base'] + packages) + '\n  build-tool-depends: alex:alex\n')
    (project / 'Main.hs').write_text('main :: IO ()\nmain = pure ()\n')
    shutil.copyfile(DEPS / 'cabal.project', project / 'cabal.project')
    copy_lock('cabal.project.freeze', project / 'cabal.project.freeze')
    # HTTPS is required even for bootstrap of Hackage's signed index.
    config = Path.home() / '.config/cabal/config'
    if not config.exists():
        config = Path.home() / '.cabal/config'
    if not config.exists():
        run('cabal', 'user-config', 'init')
        config = next(p for p in (Path.home() / '.config/cabal/config', Path.home() / '.cabal/config') if p.exists())
    config.write_text(config.read_text().replace('http://hackage.haskell.org/', 'https://hackage.haskell.org/'))
    run('cabal', 'update')
    run('cabal', 'build', 'all', '-j4', cwd=project)
    run('cabal', 'freeze', cwd=project)
    if not RESOLVE and (project / 'cabal.project.freeze').read_bytes() != (DEPS / 'cabal.project.freeze').read_bytes():
        raise RuntimeError('Haskell lock changed')
    plan = json.loads((project / 'dist-newstyle/cache/plan.json').read_text())
    # Only expose library units in the solved closure; never invoke Cabal for submissions.
    units = {u['pkg-name']: u['id'] for u in plan['install-plan']
             if u.get('component-name', 'lib') == 'lib' and u['pkg-name'] in CATALOG['haskell']}
    db = next((target / 'store').glob('ghc-*/package.db'))
    (target / 'bin').mkdir(exist_ok=True)
    shutil.copyfile(next((target / 'store').glob('ghc-*/alex-*/bin/alex')), target / 'bin/alex')
    (target / 'bin/alex').chmod(0o755)
    (target / 'compile.args').write_text('\n'.join(['-clear-package-db', '-global-package-db',
        '-package-db', str(db), *[arg for package in CATALOG['haskell'] if package != 'alex'
                                  for arg in (('-package-id', units[package]) if package in units
                                              else ('-package', package))]]) + '\n')
    for name in ('cabal.project', 'cabal.project.freeze'):
        save_lock(project / name)
    save_lock(project / 'dist-newstyle/cache/plan.json', 'haskell-plan.json')
    # Record source digests without shipping the large Hackage index to each worker.
    source_hashes = {}
    for cache in (Path.home() / '.cache/cabal/packages', Path.home() / '.cabal/packages'):
        if cache.exists():
            for path in cache.rglob('*.tar.gz'):
                if path.name.startswith('00-index') or path.name.startswith('01-index'):
                    continue
                with path.open('rb') as f:
                    source_hashes[str(path.relative_to(cache))] = hashlib.file_digest(f, 'sha256').hexdigest()
    (ROOT / 'build-manifest/additions/haskell-source-sha256.json').write_text(json.dumps(source_hashes, indent=2) + '\n')
    if not RESOLVE and source_hashes != json.loads((DEPS / 'haskell-source-sha256.json').read_text()):
        raise RuntimeError('Haskell source archive checksums changed')


def golf():
    target = ROOT / 'golf'
    target.mkdir(exist_ok=True)
    for name in ('clay', 'bc', 'brainfuck', 'whitespace', 'nibbles', 'memoize', 'murmur-hash', 'dlist'):
        unpack(name + '.tar.gz', WORK / name)
    # GCC 16 no longer includes cassert transitively through bits/stdc++.h.
    run(ROOT / 'gcc/bin/g++', '-std=c++17', '-O2', '-include', 'cassert', '-Wl,-rpath,' + str(ROOT / 'gcc/lib64'),
        WORK / 'clay/clay.cpp', '-o', target / 'clay')
    run('./configure', cwd=WORK / 'bc')
    run('make', '-C', 'lib', '-j4', cwd=WORK / 'bc')
    run('make', '-C', 'dc', '-j4', cwd=WORK / 'bc')
    shutil.copy2(WORK / 'bc/dc/dc', target / 'dc')
    # Pipe-based interactors must see pending output before the next input read.
    source = WORK / 'brainfuck/tritium/bfi.c'
    text = source.read_text()
    before_read = 'else for(;;) {\n\tif (enable_trace || debug_mode) fflush(stdout);'
    assert text.count(before_read) == 1
    source.write_text(text.replace(before_read, 'else for(;;) {\n\tfflush(stdout);'))
    run('make', '-j4', 'DO_LIBDL=', 'DO_TCC=', 'DO_LIGHT=', cwd=WORK / 'brainfuck/tritium')
    shutil.copy2(WORK / 'brainfuck/tritium/bfi.out', target / 'tritium')
    # The base runtime omits Cargo; use the same pinned Rust distribution for the builder.
    unpack('golf-rust.tar.xz', WORK / 'golf-rust')
    run('sh', 'install.sh', '--components=cargo', '--prefix=' + str(WORK / 'cargo'),
        '--disable-ldconfig', cwd=WORK / 'golf-rust')
    shutil.copy2(DEPS / 'golf/Cargo.lock', WORK / 'whitespace/Cargo.lock')
    env = dict(os.environ, RUSTC=str(ROOT / 'rust/bin/rustc'), CARGO_HOME=str(WORK / 'cargo-home'))
    run(WORK / 'cargo/bin/cargo', 'build', '--release', '--locked', cwd=WORK / 'whitespace', env=env)
    shutil.copy2(WORK / 'whitespace/target/release/wsc', target / 'wsc')
    save_lock(DEPS / 'golf/Cargo.lock', 'golf-Cargo.lock')
    libraries = target / 'nibbles-lib'
    shutil.copytree(WORK / 'memoize/src', libraries, dirs_exist_ok=True)
    shutil.copytree(WORK / 'murmur-hash/Data', libraries / 'Data', dirs_exist_ok=True)
    shutil.copytree(WORK / 'dlist/Data', libraries / 'Data', dirs_exist_ok=True)
    db = next((ROOT / 'haskell-deps/store').glob('ghc-*/package.db'))
    (target / 'nibbles.args').write_text('\n'.join(['-clear-package-db', '-global-package-db',
        '-package-db', str(db), '-package', 'split', '-L' + str(ROOT / 'addition-native/lib')]) + '\n')
    # Upstream's GHC 9.8+ compatibility fix for Template Haskell binders.
    th = libraries / 'Data/Function/Memoize/TH.hs'
    text = th.read_text()
    assert text.count('COMPAT_TY_VAR_BNDR(())') == 1
    th.write_text(text.replace('COMPAT_TY_VAR_BNDR(())', 'COMPAT_TY_VAR_BNDR(BndrVis)'))
    source = WORK / 'nibbles/nibbles.hs'
    text = source.read_text()
    # Flush generated programs for interactive judging too.
    assert text.count(' hSetEncoding stdout char8') == 1
    source.write_text(text.replace(' hSetEncoding stdout char8', ' hSetBuffering stdout NoBuffering\\n\\\n   \\ hSetEncoding stdout char8'))
    run(ROOT / 'ghc/bin/ghc', '-O', '-j1', '-i' + str(libraries),
        '@' + str(target / 'nibbles.args'), 'nibbles.hs', '-o', target / 'nibbles', cwd=WORK / 'nibbles')
    for name in ('compile-clay.sh', 'compile-nibbles.sh'):
        shutil.copy2(DEPS / 'golf' / name, target / name)
    for name, license in (('clay', 'LICENSE'), ('bc', 'COPYING'), ('brainfuck', 'LICENSE'),
                          ('whitespace', 'LICENSE'), ('nibbles', 'LICENSE.txt'),
                          ('memoize', 'LICENSE'), ('murmur-hash', 'LICENSE'), ('dlist', 'license.md')):
        shutil.copy2(WORK / name / license, target / (name + '-LICENSE'))


def goose():
    # Verify the input here too, so this step can extend an already deployed runtime tree.
    item = json.loads((INPUT / 'sources.json').read_text())['files']['goose.tar.gz']
    with (INPUT / 'goose.tar.gz').open('rb') as f:
        if hashlib.file_digest(f, 'sha256').hexdigest() != item['sha256']:
            raise RuntimeError('Goose archive checksum changed')
    # The release finds its standard library, TinyCC files and built-in packages beside the binary.
    target = ROOT / 'goose'
    unpack('goose.tar.gz', target)
    for name in ('docs', 'samples', 'vscode'):
        shutil.rmtree(target / name)
    run('chmod', '-R', 'a+rX', target)
    # Build a program the way submissions are built, so a release the judge's GCC cannot use stops here.
    probe = WORK / 'goose-probe'
    probe.mkdir(exist_ok=True)
    (probe / 'main.goose').write_text('fn main() { print(3); }\n')
    run(target / 'goose', 'build', '--standalone', '--cc', ROOT / 'gcc/bin/gcc', '-O2', probe / 'main.goose',
        '-o', probe / 'main', env=dict(os.environ, GOOSE_HOME=str(probe / 'home')))
    if subprocess.check_output([probe / 'main'], text=True) != '3\n':
        raise RuntimeError('Goose probe printed the wrong output')


def finish():
    save_lock(INPUT / 'sources.json')
    save_lock(DEPS / 'catalog.json')
    # Keep only runtime data from the newly built gems.
    for deps in ('ruby-deps', 'truffleruby-deps'):
        for temporary in (ROOT / deps).glob('bundle/*/*/gems/numo-openblas-*/vendor/tmp'):
            shutil.rmtree(temporary)
        for path in (ROOT / deps).rglob('*'):
            if path.is_file() and not path.is_symlink():
                if path.suffix in ('.o', '.a'):
                    path.unlink()
                elif '.so' in path.name:
                    run('strip', '--strip-debug', path)
        for info in (ROOT / deps).glob('bundle/*/*/build_info/*'):
            if info.name.startswith(('torch-', 'lightgbm-', 'rumale-')):
                info.unlink()
    # Copy native dependencies into the immutable tree, keeping existing prefixes untouched.
    native = ROOT / 'addition-native/lib'
    native.mkdir(parents=True, exist_ok=True)
    pending = list((ROOT / 'ruby').rglob('*.so*')) + list((ROOT / 'ruby-deps').rglob('*.so*'))
    pending += list((ROOT / 'haskell-deps/store').rglob('*.so*'))
    pending += list((ROOT / 'truffleruby-deps').rglob('*.so*'))
    pending += list((ROOT / 'truffleruby').rglob('*.so*')) + list(native.glob('*.so*'))
    # GEOS and BLAS are loaded through FFI/dlopen, so ldd alone cannot discover them.
    explicit = [Path('/usr/lib/x86_64-linux-gnu') / name for name in
                ('libgeos_c.so', 'libopenblas.so', 'liblapack.so', 'libgsl.so',
                 'libgslcblas.so', 'libglpk.so', 'libblas.so', 'libgmp.so')]
    pending += explicit
    # GHC also needs the unversioned linker names; the worker has no development packages.
    for source in explicit:
        shutil.copyfile(source, native / source.name)
    pending += [ROOT / 'ghc/bin/ghc', ROOT / 'ruby/bin/ruby']
    pending += [ROOT / 'golf' / name for name in ('clay', 'dc', 'tritium', 'wsc', 'nibbles')]
    pending.append(ROOT / 'goose/goose')
    seen = set()
    while pending:
        path = pending.pop()
        if str(path) in seen:
            continue
        seen.add(str(path))
        output = subprocess.run(['ldd', str(path)], text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL).stdout
        for dep in re.findall(r'=> (/\S+)', output):
            source = Path(dep)
            if str(source).startswith(str(ROOT)) or source.name in ('libc.so.6', 'libm.so.6', 'libpthread.so.0', 'libdl.so.2', 'librt.so.1'):
                continue
            dest = native / source.name
            if not dest.exists():
                shutil.copyfile(source, dest)
                pending.append(source)
    with (ROOT / 'build-manifest/additions/builder-packages.txt').open('w') as out:
        run('dpkg-query', '-W', '-f=${Package}=${Version}\n', stdout=out)
    native_hashes = {}
    for path in sorted(native.glob('*.so*')):
        with path.open('rb') as file:
            native_hashes[path.name] = hashlib.file_digest(file, 'sha256').hexdigest()
    native_lock = 'native-library-sha256.json'
    if not RESOLVE and native_hashes != json.loads((DEPS / native_lock).read_text()):
        raise RuntimeError('Native dependency checksums changed; update the reviewed lock first')
    (ROOT / 'build-manifest/additions' / native_lock).write_text(json.dumps(native_hashes, indent=2) + '\n')
    # Keep inventory of vendored native objects and source cache alongside package-manager locks.
    checksums = {}
    for subtree in ('ruby-deps', 'truffleruby-deps', 'haskell-deps', 'deno-deps', 'addition-native', 'golf', 'goose'):
        for path in sorted((ROOT / subtree).rglob('*')):
            if path.is_file() and not path.is_symlink():
                with path.open('rb') as f:
                    checksums[str(path.relative_to(ROOT))] = hashlib.file_digest(f, 'sha256').hexdigest()
    (ROOT / 'build-manifest/additions/content-sha256.json').write_text(json.dumps(checksums, sort_keys=True) + '\n')
    for name in ('node', 'bun', 'deno', 'ghc', 'cabal', 'ruby', 'truffleruby', 'js-deps',
                 'deno-deps', 'ruby-deps', 'truffleruby-deps', 'haskell-deps', 'addition-native', 'golf', 'goose'):
        run('chmod', '-R', 'a+rX', ROOT / name)


if __name__ == '__main__':
    {'toolchains': toolchains, 'javascript': javascript, 'ruby-native': ruby_native,
     'ruby': ruby, 'truffleruby': lambda: ruby(True), 'haskell': haskell, 'golf': golf, 'goose': goose, 'finish': finish}[sys.argv[1]]()
