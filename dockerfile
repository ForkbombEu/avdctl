# Copyright (C) 2025 Forkbomb B.V.
# License: AGPL-3.0-only

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY . .
RUN go build -o /out/avdctl ./cmd/avdctl

FROM debian:bookworm-slim
# runtime deps (emulator toolchain expected to be bind-mounted from host or present in PATH)
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates procps && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/avdctl /usr/local/bin/avdctl
ENTRYPOINT ["avdctl"]

