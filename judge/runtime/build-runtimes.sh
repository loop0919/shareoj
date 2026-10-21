#!/usr/bin/env bash
# Build on Ubuntu 24.04 x86_64, not on the 2 GB production worker.
set -euo pipefail
runtime_root=/opt/judge-runtimes
mkdir -p "$runtime_root" /build/src
cd /build/src
unpack() { mkdir -p "$2"; tar -xf "/inputs/$1" -C "$2" --strip-components=1; }
unpack gcc.tar.xz gcc
mkdir gcc-build
cd gcc-build
../gcc/configure --prefix="$runtime_root/gcc" --enable-languages=c,c++ --disable-multilib --disable-bootstrap --disable-nls
make -j4
make install-strip
cd /build/src
unpack llvm.tar.xz "$runtime_root/llvm"
unpack python.tar.xz python
cd python
./configure --prefix="$runtime_root/python" --with-ensurepip=install
make -j4
make install
cd /build/src
unpack pypy.tar.bz2 "$runtime_root/pypy"
unpack codon.tar.gz "$runtime_root/codon"
unpack java.tar.gz "$runtime_root/java"
unpack rust.tar.xz rust
bash rust/install.sh --prefix="$runtime_root/rust" --components=rustc,cargo,rust-std-x86_64-unknown-linux-gnu --disable-ldconfig
unpack boost.tar.bz2 boost
unpack acl.tar.gz acl
mkdir -p "$runtime_root/include" "$runtime_root/java-libs" "$runtime_root/build-manifest"
cp -a boost/boost acl/atcoder "$runtime_root/include/"
cp /inputs/ac_library.jar /inputs/bifurcan.jar "$runtime_root/java-libs/"
unpack acl-python.tar.gz acl-python
"$runtime_root/python/bin/python3.14" -m pip install --only-binary=:all: -r /inputs/requirements.txt
"$runtime_root/python/bin/python3.14" -m pip freeze --all > "$runtime_root/build-manifest/cpython.txt"
"$runtime_root/pypy/bin/pypy3" -m ensurepip
awk '/^(more-itertools|sortedcontainers)==/' /inputs/requirements.txt > /build/pypy-requirements.txt
"$runtime_root/pypy/bin/pypy3" -m pip install --only-binary=:all: -r /build/pypy-requirements.txt
"$runtime_root/pypy/bin/pypy3" -m pip freeze --all > "$runtime_root/build-manifest/pypy.txt"
cp -a acl-python/atcoder "$runtime_root/python/lib/python3.14/site-packages/"
cp -a acl-python/atcoder "$runtime_root/pypy/lib/pypy3.11/site-packages/"
mkdir -p /build/rust-deps/src
cp /build/rust-Cargo.toml /build/rust-deps/Cargo.toml
cp /build/rust-Cargo.lock /build/rust-deps/Cargo.lock
printf 'fn main() {}\n' > /build/rust-deps/src/main.rs
export PATH="$runtime_root/rust/bin:$PATH"
cd /build/rust-deps
cargo build --release --locked
cargo build --release --locked --offline
cp Cargo.lock "$runtime_root/build-manifest/Cargo.lock"
cp Cargo.toml "$runtime_root/build-manifest/Cargo.toml"
cp -a target/release/deps "$runtime_root/rust/deps"
printf '%s\n' '-L' "dependency=$runtime_root/rust/deps" > "$runtime_root/rust/extern.args"
for crate in ac_library fixedbitset itertools num proconio rand rustc_hash; do
  artifacts=("$runtime_root/rust/deps/lib$crate-"*.rlib)
  test "${#artifacts[@]}" = 1
  test -f "${artifacts[0]}"
  printf '%s\n' '--extern' "$crate=${artifacts[0]}" >> "$runtime_root/rust/extern.args"
done
# Package managers belong to the build environment, not to submissions.
"$runtime_root/python/bin/python3.14" -m pip uninstall -y pip
"$runtime_root/pypy/bin/pypy3" -m pip uninstall -y pip setuptools
rm "$runtime_root/rust/bin/cargo"
cp /inputs/sources.json "$runtime_root/build-manifest/sources.json"
dpkg-query -W '-f=${Package}=${Version}\n' > "$runtime_root/build-manifest/builder-packages.txt"
find "$runtime_root" -type d -exec chmod 755 {} +
find "$runtime_root" -type f -exec chmod a+r {} +
tar -C /opt -czf /runtime.tar.gz judge-runtimes
sha256sum /runtime.tar.gz
