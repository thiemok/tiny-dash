# API

HTTP server that renders an HTMX dashboard, captures it as a screenshot, reduces it to the e-ink color palette, and serves the result as packed pixel data for the picoDevice.

## Pipeline

```
Go html/template + HTMX  ->  chromedp screenshot  ->  palette reduction  ->  pixel packing  ->  HTTP response
```

Palette reduction depends on the requested `colors`. The template paints only with the six
Spectra 6 colors (black, white, yellow, red, blue, green; enforced by
`internal/dashboard/palette_test.go`). When the requested palette includes all of them, each pixel
is snapped to its nearest palette color, which keeps text and edges crisp. With a smaller palette
the screenshot is Floyd-Steinberg dithered instead, so colors the display lacks are approximated.

Chrome renders text without antialiasing through the fontconfig override in
[`fontconfig/fonts.conf`](fontconfig/fonts.conf), selected by `FONTCONFIG_FILE` in the Docker image
and in the flatpak wrapper. Set the same variable when pointing `CHROME_PATH` at another binary.

## Running

```sh
pnpm nx run api:serve
```

This uses the flatpak-installed Chrome by default. To use a different Chrome binary:

```sh
CHROME_PATH=/usr/bin/chromium pnpm nx run api:serve
```

## Endpoints

| Endpoint | Description |
|---|---|
| `GET /dashboard` | Interactive HTML dashboard (with HTMX auto-refresh) |
| `GET /api/dashboard/image` | Packed e-ink binary data (consumed by picoDevice) |
| `GET /api/dashboard/preview` | PNG preview of exactly what the panel gets (for debugging) |
| `GET /api/hello` | Health check |

### Query Parameters

| Parameter | Required | Description |
|---|---|---|
| `width` | yes | Image width in pixels |
| `height` | yes | Image height in pixels |
| `colorDepth` | image only | Bits per pixel (e.g. 4) |
| `colors` | yes | Comma-separated color indices from the palette |

### Color Palette

| Index | Color |
|---|---|
| 0 | Black |
| 1 | White |
| 2 | Yellow |
| 3 | Red |
| 4 | Orange |
| 5 | Blue |
| 6 | Green |

## Preview Examples

7-color display (800x480):
<http://localhost:8080/api/dashboard/preview?width=800&height=480&colors=0,1,2,3,4,5,6>

Impression 5.7" — black, white, red, yellow, blue, green (no orange):
<http://localhost:8080/api/dashboard/preview?width=600&height=448&colors=0,1,2,3,5,6>

Black and white only:
<http://localhost:8080/api/dashboard/preview?width=800&height=480&colors=0,1>

Black, white and red:
<http://localhost:8080/api/dashboard/preview?width=400&height=300&colors=0,1,3>

## Observability

Traces, metrics and logs are exported via OpenTelemetry (OTLP) when `OTEL_EXPORTER_OTLP_ENDPOINT` is set;
otherwise logs go to stdout only. Conventions, configuration and local verification steps are in
[docs/observability.md](docs/observability.md).

## Building

```sh
pnpm nx run api:build
```

Produces a binary at `api/dist/api`.
