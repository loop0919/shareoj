FROM ubuntu:24.04@sha256:224a1869083a311ef3f13648a154ba79832fbef6364d31493642ca03082da254
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends build-essential ca-certificates curl xz-utils bzip2 libgmp-dev libmpfr-dev libmpc-dev libssl-dev libffi-dev zlib1g-dev libbz2-dev liblzma-dev libsqlite3-dev libreadline-dev libzstd-dev libncurses-dev libxml2 libedit2 python3 python3-pip pkg-config
COPY .build/runtime-inputs /inputs
COPY runtime/build-runtimes.sh runtime/deps/rust/rust-Cargo.toml runtime/deps/rust/rust-Cargo.lock /build/
RUN bash /build/build-runtimes.sh
