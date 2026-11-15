#!/usr/bin/env python3
"""Build the additional runtimes in Ubuntu; RESOLVE=1 is only for updating locks."""
import hashlib
import json
import os
from pathlib import Path
import re
import resource
import shutil
import subprocess
import sys
import tomllib
import urllib.request
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


def fetch(url):
    with urllib.request.urlopen(urllib.request.Request(url, headers={'User-Agent': 'OpenOJ-runtime-build'}),
                                timeout=60) as response:
        return response.read()


def resolve_quicklisp(libraries):
    """Each Quicklisp release the listed systems need, from the pinned dist."""
    base = 'https://beta.quicklisp.org/dist/quicklisp/' + libraries['quicklisp'] + '/'
    index = {name: [line.split() for line in fetch(base + name).decode().splitlines() if not line.startswith('#')]
             for name in ('systems.txt', 'releases.txt')}
    # systems.txt: project, system file, system, dependencies. releases.txt: project, URL, ...
    systems = {fields[2]: fields for fields in index['systems.txt']}
    urls = {fields[0]: fields[1] for fields in index['releases.txt']}
    # ASDF also loads build_systems while loading the libraries (cl-unicode generates its tables).
    lock, seen, pending = {}, set(), [*libraries['systems'], *libraries['build_systems']]
    while pending:
        name = pending.pop()
        # Systems outside the dist (ASDF, SBCL's contribs) ship with SBCL.
        if name in seen or name not in systems:
            continue
        seen.add(name)
        pending += systems[name][3:]
        project = systems[name][0]
        lock[project] = dict(url=urls[project].replace('http://', 'https://', 1))
    for release in lock.values():
        release['sha256'] = hashlib.sha256(fetch(release['url'])).hexdigest()
    return dict(sorted(lock.items()))


def sbcl():
    # Verify the input here too, so this step can extend an already deployed runtime tree.
    item = json.loads((INPUT / 'sources.json').read_text())['files']['sbcl.tar.bz2']
    with (INPUT / 'sbcl.tar.bz2').open('rb') as f:
        if hashlib.file_digest(f, 'sha256').hexdigest() != item['sha256']:
            raise RuntimeError('SBCL archive checksum changed')
    target = ROOT / 'sbcl'
    unpack('sbcl.tar.bz2', WORK / 'sbcl')
    run('sh', 'install.sh', '--prefix=' + str(target), cwd=WORK / 'sbcl')
    shutil.rmtree(target / 'share')
    shutil.copy2(WORK / 'sbcl/COPYING', target / 'COPYING')
    # Fetch each Quicklisp release by its locked URL and SHA-256; the Quicklisp client never runs.
    libraries = json.loads((DEPS / 'sbcl/libraries.json').read_text())
    lock = resolve_quicklisp(libraries) if RESOLVE else json.loads((DEPS / 'sbcl/quicklisp.lock.json').read_text())
    (target / 'quicklisp.lock.json').write_text(json.dumps(lock, indent=2) + '\n')
    # The archives keep the libraries' licences and sources beside the image built from them.
    (target / 'sources').mkdir()
    tree = WORK / 'sbcl-libraries'
    tree.mkdir(exist_ok=True)
    for project, release in lock.items():
        archive = target / 'sources' / release['url'].rsplit('/', 1)[1]
        archive.write_bytes(fetch(release['url']))
        with archive.open('rb') as f:
            if hashlib.file_digest(f, 'sha256').hexdigest() != release['sha256']:
                raise RuntimeError('Quicklisp archive checksum changed: ' + project)
        run('tar', '-xzf', archive, '-C', tree)
    forms = ['(require :asdf)',
             '(asdf:initialize-source-registry \'(:source-registry (:tree "%s/") :ignore-inherited-configuration))' % tree,
             *['(require :%s)' % name for name in libraries['contribs']],
             '(asdf:load-systems %s)' % ' '.join(json.dumps(name) for name in libraries['systems'])]
    # gsll's groveller needs gcc and the GSL and libffi headers here. The image reopens libgmp, GSL and libffi
    # by name each time it starts, so the build and every run take them from addition-native.
    env = dict(os.environ, XDG_CACHE_HOME=str(WORK / 'sbcl-cache'), LD_LIBRARY_PATH=str(ROOT / 'addition-native/lib'))
    # The saved image keeps these sizes; the dynamic space must exceed every memory limit so that running out is MLE.
    run(target / 'bin/sbcl', '--dynamic-space-size', '1GB', '--control-stack-size', '512MB',
        '--non-interactive', '--no-sysinit', '--no-userinit', *[arg for form in forms for arg in ('--eval', form)],
        '--load', DEPS / 'sbcl/save-image.lisp', env=env)
    shutil.copy2(DEPS / 'sbcl/compile.lisp', target / 'compile.lisp')
    run('chmod', '-R', 'a+rX', target)
    # Build and run a program the way submissions are, so an image that cannot compile stops here.
    probe = WORK / 'sbcl-probe'
    probe.mkdir(exist_ok=True)
    judge = [target / 'bin/sbcl-judge', '--script']
    (probe / 'main.lisp').write_text('(format t "~a~%" (+ 1 2))\n')
    run(*judge, target / 'compile.lisp', probe / 'main.lisp', probe / 'main', env=env)
    if subprocess.check_output([*judge, probe / 'main'], text=True, env=env) != '3\n':
        raise RuntimeError('SBCL probe printed the wrong output')
    # SBCL still writes a fasl after a compile-time error; compile.lisp must reject it.
    (probe / 'error.lisp').write_text('(defmacro fails () (error "expansion"))\n(fails)\n')
    if subprocess.run([*judge, target / 'compile.lisp', probe / 'error.lisp', probe / 'error'], env=env,
                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
        raise RuntimeError('SBCL compiled a program with a compile-time error')


def fsharp():
    # C#'s pinned SDK archive; verify it here too, so this step can extend an already deployed runtime tree.
    item = json.loads((INPUT / 'sources.json').read_text())['files']['dotnet.tar.gz']
    with (INPUT / 'dotnet.tar.gz').open('rb') as f:
        if hashlib.file_digest(f, 'sha256').hexdigest() != item['sha256']:
            raise RuntimeError('.NET SDK archive checksum changed')
    # The deployed dotnet/ has no SDK. Take only the F# compiler and the reference assemblies of the deployed runtime.
    sdk = WORK / 'dotnet-sdk'
    unpack('dotnet.tar.gz', sdk, strip=False)
    compiler = sdk / 'sdk' / item['version'] / 'FSharp'
    runtime = next((ROOT / 'dotnet/shared/Microsoft.NETCore.App').iterdir()).name
    tools, libraries = ROOT / 'fsharp-tools', ROOT / 'fsharp-libs'
    shutil.copytree(compiler, tools)
    (tools / 'ref').mkdir()
    for path in (sdk / 'packs/Microsoft.NETCore.App.Ref' / runtime / 'ref/net10.0').glob('*.dll'):
        shutil.copy2(path, tools / 'ref')
    # Restore the libraries by the lock with the unpacked SDK; MSBuild never runs for submissions.
    project = WORK / 'fsharp'
    (WORK / 'fsharp-home').mkdir(parents=True, exist_ok=True)
    project.mkdir(exist_ok=True)
    shutil.copyfile(DEPS / 'fsharp/main.fsproj', project / 'main.fsproj')
    copy_lock('fsharp/packages.lock.json', project / 'packages.lock.json')
    (project / 'main.fs').write_text('printfn "%d" 3\n')
    env = dict(os.environ, HOME=str(WORK / 'fsharp-home'), DOTNET_CLI_TELEMETRY_OPTOUT='1', DOTNET_NOLOGO='1')
    run(sdk / 'dotnet', 'restore', 'main.fsproj', *([] if RESOLVE else ['--locked-mode']), '--packages', WORK / 'nuget',
        '--source', 'https://api.nuget.org/v3/index.json', cwd=project, env=env)
    run(sdk / 'dotnet', 'build', 'main.fsproj', '--no-restore', '--disable-build-servers', '-c', 'Release',
        '-o', WORK / 'fsharp-out', cwd=project, env=env)
    lock = json.loads((project / 'packages.lock.json').read_text())['dependencies']['net10.0']
    shutil.copyfile(project / 'packages.lock.json', tools / 'packages.lock.json')
    libraries.mkdir()
    shutil.copy2(WORK / 'fsharp-out/main.runtimeconfig.json', libraries)
    for path in (WORK / 'fsharp-out').glob('*.dll'):
        if path.name != 'main.dll':
            shutil.copy2(path, libraries)
    # runtimes.py lists one DLL per locked package; a new transitive package must be added there too.
    if {path.stem for path in libraries.glob('*.dll')} != set(lock):
        raise RuntimeError('F# library DLLs no longer match the locked packages')
    # The SDK's FSharp.Core is the locked version, compiled ahead of time, so programs start faster with it.
    if not (compiler / 'library-packs' / ('FSharp.Core.' + lock['FSharp.Core']['resolved'] + '.nupkg')).exists():
        raise RuntimeError('The locked FSharp.Core is not the SDK version')
    shutil.copy2(compiler / 'FSharp.Core.dll', libraries / 'FSharp.Core.dll')
    for name in ('MathNet.Numerics.dll', 'ac-library-csharp.dll'):
        if (libraries / name).read_bytes() != (ROOT / 'dotnet-libs' / name).read_bytes():
            raise RuntimeError(name + ' differs from the copy C# uses')
    (tools / 'licenses').mkdir()
    for package in (WORK / 'nuget').iterdir():
        for path in package.glob('*/*'):
            if path.suffix == '.nuspec' or path.name.lower().startswith(('license', 'copying', 'notice')):
                (tools / 'licenses' / package.name).mkdir(exist_ok=True)
                shutil.copy2(path, tools / 'licenses' / package.name)
    references = sorted((tools / 'ref').glob('*.dll')) + sorted(libraries.glob('*.dll'))
    # fsc copies FSharp.Core.dll beside its output unless told not to; the box gets it from fsharp-libs.
    (tools / 'compile.args').write_text('\n'.join([
        '--targetprofile:netcore', '--noframework', '--simpleresolution', '--target:exe', '--optimize+', '--debug-',
        '--nologo', '--nocopyfsharpcore', *['-r:' + str(path) for path in references],
        '--out:/box/main.dll', '/box/main.fs']) + '\n')
    run('chmod', '-R', 'a+rX', tools, libraries)
    # Build and run a program the way submissions are, with the deployed runtime.
    probe = WORK / 'fsharp-probe'
    probe.mkdir(exist_ok=True)
    (probe / 'main.fs').write_text('printfn "%d" (1 + 2)\n')
    (probe / 'compile.args').write_text((tools / 'compile.args').read_text().replace('/box/', str(probe) + '/'))
    env = dict(env, DOTNET_gcServer='0', DOTNET_PROCESSOR_COUNT='1')
    run(ROOT / 'dotnet/dotnet', tools / 'fsc.dll', '@' + str(probe / 'compile.args'), env=env)
    for path in libraries.iterdir():
        if path.is_file():
            shutil.copy2(path, probe)
    if subprocess.check_output([ROOT / 'dotnet/dotnet', probe / 'main.dll'], text=True, env=env) != '3\n':
        raise RuntimeError('F# probe printed the wrong output')


def gleam():
    # Verify the inputs here too, so this step can extend an already deployed runtime tree.
    files = json.loads((INPUT / 'sources.json').read_text())['files']
    for name in ('otp.tar.gz', 'gleam.tar.gz'):
        with (INPUT / name).open('rb') as f:
            if hashlib.file_digest(f, 'sha256').hexdigest() != files[name]['sha256']:
                raise RuntimeError(name + ' checksum changed')
    erlang, tools, project = ROOT / 'erlang', ROOT / 'gleam', ROOT / 'gleam-project'
    # hex.pm's build runs from any root once Install has written it into the launchers.
    unpack('otp.tar.gz', erlang)
    run('./Install', '-minimal', erlang, cwd=erlang)
    unpack('gleam.tar.gz', tools, strip=False)
    shutil.copy2(DEPS / 'gleam/compile.sh', tools / 'compile.sh')
    # Fetch the locked Hex packages and compile them once; a submission compiles only its own module.
    project.mkdir()
    shutil.copyfile(DEPS / 'gleam/gleam.toml', project / 'gleam.toml')
    copy_lock('gleam/manifest.toml', project / 'manifest.toml')
    (project / 'src').mkdir()
    (project / 'src/main.gleam').write_text('pub fn main() {\n  Nil\n}\n')
    home = WORK / 'gleam-home'
    env = dict(os.environ, PATH=str(erlang / 'bin') + ':' + os.environ['PATH'], HOME=str(home))
    run(tools / 'gleam', 'deps', 'update' if RESOLVE else 'download', cwd=project, env=env)
    if not RESOLVE and (project / 'manifest.toml').read_bytes() != (DEPS / 'gleam/manifest.toml').read_bytes():
        raise RuntimeError('Gleam lock changed')
    # Gleam caches each Hex tarball under its outer checksum; check the bytes against the lock ourselves.
    for package in tomllib.loads((project / 'manifest.toml').read_text())['packages']:
        with (home / '.cache/gleam/hex/hexpm/packages' / (package['outer_checksum'] + '.tar')).open('rb') as f:
            if hashlib.file_digest(f, 'sha256').hexdigest().upper() != package['outer_checksum']:
                raise RuntimeError('Hex package checksum changed: ' + package['name'])
    run(tools / 'gleam', 'build', cwd=project, env=env)
    # Keep the dependencies and the entry module gleam generates for the package (judge@@main), not the placeholder.
    (project / 'src/main.gleam').unlink()
    for path in (project / 'build/dev/erlang/judge').glob('*/main.*'):
        path.unlink()
    # The run puts one directory on the code path: erl looks through every -pa directory for each module it loads.
    ebin = project / 'ebin'
    ebin.mkdir()
    packages = project / 'build/dev/erlang'
    if any(packages.glob('*/priv')):
        raise RuntimeError('A Gleam package has a priv directory, which one shared ebin cannot serve')
    for path in sorted(packages.glob('*/ebin/*')):
        if (ebin / path.name).exists():
            raise RuntimeError('Two Gleam packages ship ' + path.name)
        shutil.copy2(path, ebin)
    run('chmod', '-R', 'a+rX', erlang, tools, project)
    # Build and run a program the way submissions are: under the sandbox's file size limits (32 MiB compiling,
    # 16 MiB + 1 KiB running) and with a dead proxy for Hex, so a template that would fetch packages, a JIT
    # mapping the limit kills or a launcher that cuts off stdin stops here.
    def file_limit(kib):
        return lambda: resource.setrlimit(resource.RLIMIT_FSIZE, (kib * 1024, kib * 1024))
    probe = WORK / 'gleam-probe'
    probe.mkdir(exist_ok=True)
    (probe / 'compile.sh').write_text((tools / 'compile.sh').read_text().replace('/box/', str(probe) + '/'))
    offline = dict(os.environ, HOME=str(probe), HTTP_PROXY='http://127.0.0.1:9', HTTPS_PROXY='http://127.0.0.1:9',
                   ERL_CRASH_DUMP_SECONDS='0')
    (probe / 'main.gleam').write_text('import gleam/int\nimport gleam/io\nimport gleam/list\nimport gleam/string\n'
        'import gleam/yielder\nimport stdin\n\npub fn main() {\n  stdin.read_lines()\n  |> yielder.to_list\n'
        '  |> list.flat_map(fn(line) { string.split(string.trim(line), " ") })\n'
        '  |> list.filter_map(int.parse)\n  |> int.sum\n  |> int.to_string\n  |> io.println\n}\n')
    run('sh', probe / 'compile.sh', env=offline, preexec_fn=file_limit(32768))
    output = subprocess.run([erlang / 'bin/erl', '+S', '1:1', '+SDcpu', '1:1', '+SDio', '1', '+A', '1',
                             '+sbwt', 'none', '+sbwtdcpu', 'none', '+sbwtdio', 'none', '-mode', 'minimal',
                             '-boot', 'no_dot_erlang', '+JMsingle', 'true', '+P', '262144', '+MMmcs', '1',
                             '-noshell', '-pa', probe, '-pa', ebin, '-eval', 'judge@@main:run(main)', '-extra'],
                            input='1 2\n', text=True,
                            stdout=subprocess.PIPE, env=offline, preexec_fn=file_limit(16385), check=True).stdout
    if output != '3\n':
        raise RuntimeError('Gleam probe printed the wrong output')
    # compile.sh must reject a module without pub fn main(), which would otherwise only fail when it runs.
    shutil.rmtree(probe / 'project')
    (probe / 'main.gleam').write_text('pub fn helper() {\n  Nil\n}\n')
    if subprocess.run(['sh', probe / 'compile.sh'], env=offline, preexec_fn=file_limit(32768),
                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
        raise RuntimeError('Gleam compiled a program without main')


def tree_sha256(directory):
    """One SHA-256 over the paths and contents of a checked-out tree, without its .git."""
    lines = []
    for path in sorted(directory.rglob('*')):
        name = path.relative_to(directory)
        if name.parts[0] == '.git':
            continue
        if path.is_symlink():
            lines.append('%s -> %s\n' % (name, os.readlink(path)))
        elif path.is_file():
            with path.open('rb') as f:
                lines.append('%s %s\n' % (name, hashlib.file_digest(f, 'sha256').hexdigest()))
    return hashlib.sha256(''.join(lines).encode()).hexdigest()


def fix():
    # Verify the input here too, so this step can extend an already deployed runtime tree.
    item = json.loads((INPUT / 'sources.json').read_text())['files']['fix']
    with (INPUT / 'fix').open('rb') as f:
        if hashlib.file_digest(f, 'sha256').hexdigest() != item['sha256']:
            raise RuntimeError('Fix binary checksum changed')
    tools, project = ROOT / 'fix', ROOT / 'fix-project'
    tools.mkdir()
    shutil.copyfile(INPUT / 'fix', tools / 'fix')
    (tools / 'fix').chmod(0o755)
    shutil.copy2(DEPS / 'fix/compile.sh', tools / 'compile.sh')
    # Clone the locked commit of each library; a submission's build finds them installed and fetches nothing.
    project.mkdir()
    shutil.copyfile(DEPS / 'fix/fixproj.toml', project / 'fixproj.toml')
    copy_lock('fix/fixdeps.lock', project / 'fixdeps.lock')
    # Fix compiles its runtime and links with the gcc on PATH, and four libraries make a lib.o; all use the judge's GCC.
    env = dict(os.environ, PATH=str(ROOT / 'gcc/bin') + ':' + os.environ['PATH'], CC='gcc', HOME=str(WORK / 'fix-home'))
    run(tools / 'fix', 'deps', 'update' if RESOLVE else 'install', cwd=project, env=env)
    if not RESOLVE and (project / 'fixdeps.lock').read_bytes() != (DEPS / 'fix/fixdeps.lock').read_bytes():
        raise RuntimeError('Fix lock changed')
    # fix checks out each locked commit; check that here, and pin the checked-out files by SHA-256 besides git's SHA-1.
    lock = tomllib.loads((project / 'fixdeps.lock').read_text())['dependencies']
    digests = {}
    for dep in lock:
        source = project / dep['path']
        if (source / '.git/HEAD').read_text().strip() != dep['git']['rev']:
            raise RuntimeError('Fix dependency is not at its locked commit: ' + dep['name'])
        digests[dep['name']] = tree_sha256(source)
    (tools / 'fixdeps.sha256.json').write_text(json.dumps(digests, indent=2, sort_keys=True) + '\n')
    if not RESOLVE and digests != json.loads((DEPS / 'fix/fixdeps.sha256.json').read_text()):
        raise RuntimeError('Fix dependency contents changed')
    # Type checking covers every module of every dependency and is cached in the project, so one build of a program
    # that imports them all leaves a submission only its own module to check. The objects are not kept (AtCoder's way).
    modules = []
    for dep in lock:
        source = project / dep['path']
        for name in tomllib.loads((source / 'fixproj.toml').read_text())['build']['files']:
            modules.append(re.search(r'^module ([\w.]+);', (source / name).read_text(), re.M).group(1))
    (project / 'main.fix').write_text('module Main;\n\n' + ''.join('import %s;\n' % name for name in modules) +
                                      '\nmain : IO ();\nmain = println("3");\n')
    run(tools / 'fix', 'build', '-O', 'none', cwd=project, env=env)
    if subprocess.check_output([project / 'a.out'], text=True) != '3\n':
        raise RuntimeError('Fix warm-up program printed the wrong output')
    for path in (project / 'main.fix', project / 'a.out'):
        path.unlink()
    for path in (project / '.fixlang/intermediate', project / '.fixlang/cache/units'):
        shutil.rmtree(path)
    run('chmod', '-R', 'a+rX', tools, project)
    # Build and run a program the way submissions are, under the compile's file size limit.
    probe = WORK / 'fix-probe'
    probe.mkdir(exist_ok=True)
    (probe / 'compile.sh').write_text((tools / 'compile.sh').read_text().replace('/box/', str(probe) + '/'))
    (probe / 'main.fix').write_text('module Main;\n\nimport CPLib.IO;\n\nmain : IO ();\n'
                                    'main = println((*read_i + *read_i).to_string);\n')
    run('sh', probe / 'compile.sh', env=dict(env, HOME=str(probe)),
        preexec_fn=lambda: resource.setrlimit(resource.RLIMIT_FSIZE, (32 << 20, 32 << 20)))
    output = subprocess.run([probe / 'main'], input='1 2\n', text=True, stdout=subprocess.PIPE, check=True,
                            env=dict(env, LD_LIBRARY_PATH=str(ROOT / 'addition-native/lib'))).stdout
    if output != '3\n':
        raise RuntimeError('Fix probe printed the wrong output')
    # The copy keeps modification times, so a build that resolved the lock again, cloned a library again (both need
    # the network) or rebuilt a library's lib.o shows here.
    for name in ['fixdeps.lock', *[dep['path'] + '/' + name for dep in lock for name in ('.git/HEAD', 'lib.o')]]:
        template, copy = project / name, probe / 'project' / name
        if template.exists() and copy.stat().st_mtime_ns != template.stat().st_mtime_ns:
            raise RuntimeError('A Fix build changed ' + name)


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
    pending.append(ROOT / 'sbcl/bin/sbcl-judge')
    pending += list((ROOT / 'erlang').glob('erts-*/bin/beam.smp'))
    pending.append(ROOT / 'fix/fix')
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
    for subtree in ('ruby-deps', 'truffleruby-deps', 'haskell-deps', 'deno-deps', 'addition-native', 'golf', 'goose', 'sbcl',
                    'fsharp-tools', 'fsharp-libs', 'erlang', 'gleam', 'gleam-project', 'fix', 'fix-project'):
        for path in sorted((ROOT / subtree).rglob('*')):
            if path.is_file() and not path.is_symlink():
                with path.open('rb') as f:
                    checksums[str(path.relative_to(ROOT))] = hashlib.file_digest(f, 'sha256').hexdigest()
    (ROOT / 'build-manifest/additions/content-sha256.json').write_text(json.dumps(checksums, sort_keys=True) + '\n')
    for name in ('node', 'bun', 'deno', 'ghc', 'cabal', 'ruby', 'truffleruby', 'js-deps',
                 'deno-deps', 'ruby-deps', 'truffleruby-deps', 'haskell-deps', 'addition-native', 'golf', 'goose', 'sbcl',
                 'fsharp-tools', 'fsharp-libs', 'erlang', 'gleam', 'gleam-project', 'fix', 'fix-project'):
        run('chmod', '-R', 'a+rX', ROOT / name)


if __name__ == '__main__':
    {'toolchains': toolchains, 'javascript': javascript, 'ruby-native': ruby_native,
     'ruby': ruby, 'truffleruby': lambda: ruby(True), 'haskell': haskell, 'golf': golf, 'goose': goose, 'sbcl': sbcl,
     'fsharp': fsharp, 'gleam': gleam, 'fix': fix, 'finish': finish}[sys.argv[1]]()
