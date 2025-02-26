# Base image
FROM debian:bookworm AS base

ARG AGAVE_VERSION=2.0.24
ARG RUST_VERSION=stable

RUN rm /bin/sh && ln -s /bin/bash /bin/sh

WORKDIR /app

RUN mkdir -p "/app/bin"

ENV PATH="/app/bin:${PATH}"

# Builder image
FROM base AS builder

RUN apt update && \
    apt-get install -y \
    build-essential \
    libssl-dev \
    pkg-config \
    protobuf-compiler \
    clang cmake curl libudev-dev  && \
    rm -rf /var/lib/apt/lists/*

RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- --default-toolchain $RUST_VERSION -y
ENV PATH="/root/.cargo/bin:${PATH}"

RUN curl https://codeload.github.com/anza-xyz/agave/tar.gz/refs/tags/v$AGAVE_VERSION | tar xvz
RUN mv /app/agave-$AGAVE_VERSION /app/agave


WORKDIR /app/agave
# Build validator
RUN cargo build --bin solana-test-validator --release
RUN cp target/release/solana-test-validator /app/bin/

# Build solana
RUN cargo build --bin solana --release
RUN cp target/release/solana /app/bin/


# Final app image
FROM base AS final

RUN apt update && \
    apt-get install -y bzip2 && \
    rm -rf /var/lib/apt/lists/*

ADD solana_artifacts/ /programs/


COPY --from=builder /app/bin/* /app/bin
