# syntax=docker/dockerfile:1

# ---- build stage --------------------------------------------------------------
# Build context is the api/ module, which is a standalone Go module with no
# replace directives to sibling modules, so it compiles in isolation.
FROM golang:1.25-bookworm AS build

WORKDIR /src

# Cache module downloads independently of source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .

# chromedp is pure Go, so a static binary (CGO disabled) is safe and portable.
# GOWORK=off ignores any workspace file; the module builds on its own.
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOWORK=off GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- runtime stage ------------------------------------------------------------
# headless-shell ships a minimal headless Chrome purpose-built for chromedp.
FROM chromedp/headless-shell:148.0.7778.97

# Fonts: headless-shell is minimal; without these, dashboard text renders as
# tofu boxes. Liberation covers Latin; Noto adds broad Unicode/symbol coverage.
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        fonts-liberation \
        fonts-noto-core \
        fonts-noto-color-emoji \
    && rm -rf /var/lib/apt/lists/*

# The renderer reads CHROME_PATH; point it at the bundled headless-shell binary.
ENV CHROME_PATH=/headless-shell/headless-shell

# Run as an unprivileged user. Chrome already runs with --no-sandbox (set in
# api/internal/render/screenshot.go), so root is not required.
RUN groupadd --system --gid 10001 app \
    && useradd --system --uid 10001 --gid app --home-dir /home/app --create-home app
ENV HOME=/home/app

COPY --from=build /out/api /usr/local/bin/api

USER app
WORKDIR /home/app

EXPOSE 8080

LABEL org.opencontainers.image.source="https://github.com/thiemok/tiny-dash" \
      org.opencontainers.image.description="tiny-dash dashboard API (renders e-ink dashboard via headless Chrome)" \
      org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/api"]
