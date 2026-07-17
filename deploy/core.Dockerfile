# Zenith core — event ingestion, stats API, console.
#
# CGO is required: DuckDB is a C++ library. That rules out a scratch/alpine
# final stage, so we build and run on Debian slim and keep the image lean by
# copying only the binary forward.

FROM golang:1.26-bookworm AS build

WORKDIR /src

# Cache module downloads separately from source changes.
COPY core/go.mod core/go.sum ./
RUN go mod download

COPY core/ ./

ENV CGO_ENABLED=1
RUN go build -ldflags="-s -w" -o /out/core ./cmd/core

FROM debian:bookworm-slim

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates \
	&& rm -rf /var/lib/apt/lists/*

# Run unprivileged: nothing here needs root.
RUN useradd --system --uid 10001 --create-home zenith

COPY --from=build /out/core /usr/local/bin/core

# The compose volume mounts here.
RUN mkdir -p /data && chown zenith:zenith /data
VOLUME /data
ENV ZENITH_DATA_DIR=/data

USER zenith
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/core"]
