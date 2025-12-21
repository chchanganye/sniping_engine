FROM golang:1.24.5-bookworm AS builder

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn
ENV GOPROXY=$GOPROXY
ENV GOSUMDB=$GOSUMDB

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM debian:bookworm-slim

ARG APT_MIRROR=mirrors.aliyun.com
RUN set -eux; \
  if [ -f /etc/apt/sources.list.d/debian.sources ]; then \
    sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" /etc/apt/sources.list.d/debian.sources; \
  elif [ -f /etc/apt/sources.list ]; then \
    sed -i "s|deb.debian.org|${APT_MIRROR}|g; s|security.debian.org|${APT_MIRROR}|g" /etc/apt/sources.list; \
  fi; \
  apt-get -o Acquire::Retries=3 update; \
  apt-get install -y --no-install-recommends ca-certificates chromium fonts-noto-cjk; \
  rm -rf /var/lib/apt/lists/*

ENV ROD_BROWSER_BIN=/usr/bin/chromium

WORKDIR /app
RUN useradd -m -u 10001 app \
  && mkdir -p /app/data \
  && chown -R app:app /app

COPY --from=builder /out/server /app/server
USER app

EXPOSE 8090
ENTRYPOINT ["/app/server", "-config", "/app/config.yaml"]
