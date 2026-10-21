#!/usr/bin/env bash
# ADR 0010 additions. Run only in the Ubuntu builder, never on the judge host.
set -euo pipefail
runtime_root=/opt/judge-runtimes
native_root=$runtime_root/nim-deps
mkdir -p /build/extension "$runtime_root/build-manifest"
cd /build/extension
unpack() { mkdir -p "$2"; tar -xf "/extension-inputs/$1" -C "$2" --strip-components=1; }

case "${1:?toolchains, native, or packages}" in
toolchains)
  unpack go.tar.gz "$runtime_root/go"
  mkdir -p "$runtime_root/dotnet"
  tar -xf /extension-inputs/dotnet.tar.gz -C "$runtime_root/dotnet"
  unpack java25.tar.gz "$runtime_root/java25"
  unpack nim.tar.gz nim
  unpack nim-csources.tar.gz nim/csources_v3
  cd nim
  make -C csources_v3 -j4
  test -x bin/nim
  bin/nim c koch
  ./koch boot -d:release
  mkdir -p "$runtime_root/nim/bin"
  cp bin/nim "$runtime_root/nim/bin/"
  cp -a lib config "$runtime_root/nim/"
  cp copying.txt "$runtime_root/nim/"
  ;;
native)
  mkdir -p native "$native_root"
  cd native
  unpack gmp.tar.xz gmp
  unpack mpfr.tar.xz mpfr
  unpack fftw.tar.gz fftw
  unpack eigen.tar.gz eigen
  unpack nim-boost.tar.bz2 boost
  unpack nim-ac-library.tar.gz ac-library
  cd gmp
  ./configure --prefix="$native_root" --enable-cxx --enable-fat
  make -j4
  make install
  cd ../mpfr
  ./configure --prefix="$native_root" --with-gmp="$native_root" LDFLAGS="-Wl,-rpath,$native_root/lib"
  make -j4
  make install
  cd ../fftw
  ./configure --prefix="$native_root" --enable-shared --disable-fortran CFLAGS='-O2 -march=x86-64 -mtune=generic'
  make -j4
  make install
  cd ../boost
  ./bootstrap.sh --prefix="$native_root" --without-libraries=mpi,graph_parallel,python
  ./b2 -j4 cxxflags='-O2 -march=x86-64 -mtune=generic' install
  cd ..
  mkdir -p "$native_root/include/eigen3"
  cp -a eigen/Eigen eigen/unsupported "$native_root/include/eigen3/"
  cp -a ac-library/atcoder "$native_root/include/"
  for package in gmp mpfr fftw boost eigen ac-library; do
    mkdir -p "$runtime_root/licenses/$package"
    find "$package" -maxdepth 1 -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'copyright*' -o -iname 'notice*' \) -exec cp '{}' "$runtime_root/licenses/$package/" \;
  done
  # Keep CPU BLAS and its runtime dependencies in the immutable runtime tree.
  for library in libopenblas.so.0 libblas.so.3 liblapack.so.3 libgfortran.so.5 libquadmath.so.0; do
    library_path=$(ldconfig -p | awk -v name="$library" '$1 == name && /x86-64/ {print $NF; exit}')
    test -n "$library_path"
    cp -L "$library_path" "$native_root/lib/$library"
  done
  ln -sf libopenblas.so.0 "$native_root/lib/libopenblas.so"
  ln -sf libblas.so.3 "$native_root/lib/libblas.so"
  ln -sf liblapack.so.3 "$native_root/lib/liblapack.so"
  ;;
packages)
  cp /extension-inputs/testlib.h "$runtime_root/include/"
  export DOTNET_CLI_TELEMETRY_OPTOUT=1 DOTNET_NOLOGO=1
  mkdir -p dotnet
  cp /build/dotnet-deps/main.csproj /build/dotnet-deps/packages.lock.json dotnet/
  printf 'class Program { static void Main() {} }\n' > dotnet/main.cs
  "$runtime_root/dotnet/dotnet" restore dotnet/main.csproj --locked-mode --packages /build/extension/nuget
  "$runtime_root/dotnet/dotnet" build dotnet/main.csproj --no-restore -c Release -o "$runtime_root/dotnet-libs"
  mkdir -p "$runtime_root/csharp-tools/ref"
  cp -a "$runtime_root/dotnet/sdk/10.0.401/Roslyn/bincore/." "$runtime_root/csharp-tools/"
  cp "$runtime_root/dotnet/packs/Microsoft.NETCore.App.Ref/10.0.12/ref/net10.0/"*.dll "$runtime_root/csharp-tools/ref/"
  {
    printf '%s\n' '-nologo' '-target:exe' '-langversion:14' '-optimize+' '-unsafe+' '-out:/box/main.dll'
    for reference in "$runtime_root/csharp-tools/ref/"*.dll "$runtime_root/dotnet-libs/MathNet.Numerics.dll" "$runtime_root/dotnet-libs/ac-library-csharp.dll"; do
      printf '%s\n' "-reference:$reference"
    done
    printf '%s\n' '/box/main.cs'
  } > "$runtime_root/csharp-tools/compile.args"
  rm "$runtime_root/dotnet-libs/main" "$runtime_root/dotnet-libs/main.dll" "$runtime_root/dotnet-libs/main.pdb"
  rm -rf "$runtime_root/dotnet/sdk" "$runtime_root/dotnet/packs" "$runtime_root/dotnet/templates" "$runtime_root/dotnet/sdk-manifests"
  cp /build/dotnet-deps/packages.lock.json "$runtime_root/build-manifest/nuget.lock.json"
  mkdir -p "$runtime_root/licenses/nuget"
  find /build/extension/nuget -type f -name '*.nuspec' -exec cp '{}' "$runtime_root/licenses/nuget/" \;
  for package in mathnet.numerics ac-library-csharp; do
    mkdir -p "$runtime_root/licenses/nuget/$package"
    find "/build/extension/nuget/$package" -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'notice*' \) -exec cp '{}' "$runtime_root/licenses/nuget/$package/" \;
  done
  mkdir -p go
  cp /build/go-deps/{go.mod,go.sum,main.go} go/
  cd go
  export GOTOOLCHAIN=local
  "$runtime_root/go/bin/go" mod download
  "$runtime_root/go/bin/go" mod verify
  "$runtime_root/go/bin/go" mod vendor
  cmp go.mod /build/go-deps/go.mod
  cmp go.sum /build/go-deps/go.sum
  mkdir -p "$runtime_root/go-deps"
  cp -a go.mod go.sum vendor "$runtime_root/go-deps/"
  cp go.mod go.sum "$runtime_root/build-manifest/"
  cd ..
  python3 /build/install-nim-packages.py
  cp /extension-inputs/sources.json "$runtime_root/build-manifest/extensions.json"
  dpkg-query -W '-f=${Package}=${Version}\n' > "$runtime_root/build-manifest/extension-builder-packages.txt"
  find "$runtime_root" -type d -exec chmod 755 {} +
  find "$runtime_root" -type f ! -perm /111 -exec chmod 644 {} +
  ;;
*) exit 2 ;;
esac
