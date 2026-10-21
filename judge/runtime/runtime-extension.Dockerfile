FROM ubuntu:24.04@sha256:224a1869083a311ef3f13648a154ba79832fbef6364d31493642ca03082da254
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates curl xz-utils bzip2 m4 patch git cmake python3 pkg-config libssl-dev libopenblas-dev liblapack-dev libicu74
# Extend the tested runtime archive without rebuilding existing compilers.
COPY .build/runtime.tar.gz /inputs/base-runtime.tar.gz
RUN echo '553586f59fe5a1a28be31505c6984a85261cb0dd418574a15285d629f6cd17eb  /inputs/base-runtime.tar.gz' | sha256sum -c - && tar -xzf /inputs/base-runtime.tar.gz -C /opt
COPY .build/extension-inputs /extension-inputs
COPY runtime/build-runtime-extensions.sh /build/
RUN bash /build/build-runtime-extensions.sh toolchains
RUN bash /build/build-runtime-extensions.sh native
COPY runtime/deps/dotnet /build/dotnet-deps
COPY runtime/deps/go /build/go-deps
COPY runtime/install-nim-packages.py runtime/deps/nim/nim-gmp-refc.patch runtime/deps/nim/bignum-refc.patch /build/
RUN bash /build/build-runtime-extensions.sh packages
RUN tar -C /opt -czf /runtime.tar.gz judge-runtimes && sha256sum /runtime.tar.gz
