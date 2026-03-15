# API

HTTP server that renders an HTMX dashboard, captures it as a screenshot, dithers it to an e-ink color palette, and serves the result as packed pixel data for the picoDevice.

## Pipeline

```
Go html/template + HTMX  ->  chromedp screenshot  ->  Floyd-Steinberg dither  ->  pixel packing  ->  HTTP response
```

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
| `GET /api/dashboard/preview` | Dithered PNG preview (for debugging) |
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

## Building

```sh
pnpm nx run api:build
```

Produces a binary at `api/dist/api`.
