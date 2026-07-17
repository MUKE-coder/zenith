# Zenith audit worker — headless Chromium SEO audits.
#
# Separate image from core by design: Chromium is ~1GB and an OOM here must not
# be able to take analytics ingestion down. A developer who doesn't want SEO
# audits simply never starts this service.

FROM golang:1.26-bookworm AS build

WORKDIR /src

COPY audit-worker/go.mod audit-worker/go.su[m] ./
RUN go mod download

COPY audit-worker/ ./

RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/worker ./cmd/worker

FROM debian:bookworm-slim

# Chromium and the fonts it needs to render real pages.
RUN apt-get update \
	&& apt-get install -y --no-install-recommends \
		ca-certificates \
		chromium \
		fonts-liberation \
	&& rm -rf /var/lib/apt/lists/*

# The SAME uid and gid as core (10001). Both services write to one shared
# volume -- core owns the databases, the worker updates the queue and writes
# results -- and a named volume takes its ownership from whichever container
# initializes it first. Different uids meant that race decided whether core
# could open its own database: usually it could not.
RUN useradd --system --uid 10001 --create-home zenith

COPY --from=build /out/worker /usr/local/bin/worker

RUN mkdir -p /data && chown zenith:zenith /data
ENV ZENITH_DATA_DIR=/data
ENV ZENITH_CHROME_PATH=/usr/bin/chromium

USER zenith

ENTRYPOINT ["/usr/local/bin/worker"]
