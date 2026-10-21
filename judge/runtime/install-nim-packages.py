#!/usr/bin/env python3
"""Install the locked source paths without running package hooks or resolvers."""
import json
from pathlib import Path
import shutil
import subprocess

root = Path('/opt/judge-runtimes')
inputs = Path('/extension-inputs')
manifest = json.loads((inputs / 'sources.json').read_text())
paths = []
for archive, entry in sorted(manifest['files'].items()):
    if 'nim_path' not in entry:
        continue
    package = root / 'nim-packages' / archive.removesuffix('.tar.gz')
    package.mkdir(parents=True, exist_ok=True)
    subprocess.run(['tar', '-xf', str(inputs / archive), '-C', str(package), '--strip-components=1'], check=True)
    if archive in ('nim-gmp.tar.gz', 'bignum.tar.gz'):
        # Nim 2 refc still requires `var T` for destructors; neo requires refc.
        patch = archive.removesuffix('.tar.gz') + '-refc.patch'
        subprocess.run(['patch', '--batch', '--fuzz=0', '-p1', '-i', '/build/' + patch], cwd=package, check=True)
        shutil.copyfile('/build/' + patch, root / 'build-manifest' / patch)
    paths.append(str(package / entry['nim_path']))
    # Nim-ACL's documented `lib` alias normally comes from its install hook.
    if archive == 'nim-acl.tar.gz':
        alias = package / 'src/lib'
        if not alias.is_symlink():
            alias.symlink_to('atcoder/extra', target_is_directory=True)

native = str(root / 'nim-deps')
gcc = str(root / 'gcc/bin/g++')
configuration = '\n# Operator-owned ADR 0010 package paths and native libraries.\n'
configuration += ''.join(f'path:"{path}"\n' for path in paths)
configuration += f'''
gcc.cpp.exe="{gcc}"
gcc.cpp.linkerexe="{gcc}"
cincludes="{native}/include"
cincludes="{native}/include/eigen3"
clibdir="{native}/lib"
passC="-march=x86-64 -mtune=generic"
passL="-Wl,-rpath,{native}/lib -Wl,-rpath,{root}/gcc/lib64"
define:blas=openblas
define:lapack=openblas
'''
with (root / 'nim/config/nim.cfg').open('a') as output:
    output.write(configuration)
shutil.copyfile(root / 'nim/config/nim.cfg', root / 'build-manifest/nim.cfg')
