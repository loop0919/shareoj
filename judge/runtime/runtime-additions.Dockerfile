FROM ubuntu:24.04@sha256:224a1869083a311ef3f13648a154ba79832fbef6364d31493642ca03082da254 AS toolchains
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates curl xz-utils bzip2 unzip git python3 pkg-config cmake libssl-dev libyaml-dev libreadline-dev zlib1g-dev libffi-dev libgmp-dev libnuma-dev libncurses-dev libopenblas-dev liblapack-dev libgsl-dev libglpk-dev libgeos-dev libz3-dev
COPY .build/runtime-adr0010.tar.gz /inputs/base.tar.gz
COPY .build/addition-inputs /inputs/additions
COPY runtime/build-runtime-additions.py /build/
COPY runtime/deps/additions /build/deps
ARG RESOLVE=0
RUN python3 /build/build-runtime-additions.py toolchains
ENV PATH="/opt/judge-runtimes/node/bin:/opt/judge-runtimes/ruby/bin:/opt/judge-runtimes/ghc/bin:/opt/judge-runtimes/cabal/bin:${PATH}"
FROM toolchains AS packages
RUN python3 /build/build-runtime-additions.py javascript
RUN python3 /build/build-runtime-additions.py ruby-native
RUN python3 /build/build-runtime-additions.py ruby
RUN python3 /build/build-runtime-additions.py truffleruby
RUN python3 /build/build-runtime-additions.py haskell
RUN python3 /build/build-runtime-additions.py golf
RUN python3 /build/build-runtime-additions.py goose
RUN python3 /build/build-runtime-additions.py finish
RUN tar -C /opt -czf /runtime.tar.gz judge-runtimes && sha256sum /runtime.tar.gz
