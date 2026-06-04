# Deploying the `api` component

The `api` service (Go + headless Chrome dashboard renderer) is released automatically on
every merge to `main`, published as a container image to GHCR, and deployed to Kubernetes
with the in-repo Helm chart.

## Pipeline at a glance

```
PR ──▶ CI (.github/workflows/ci.yml)            nx affected: lint · test · build
 │
merge to main
 │
 ▼
Release (.github/workflows/release.yml)
 ├─ nx release: version from conventional commits → api/CHANGELOG.md
 │              → tag api-v<X.Y.Z> → GitHub Release
 ├─ sync charts/api Chart.yaml (version + appVersion) → commit [skip ci]
 └─ docker build api/ → push ghcr.io/thiemok/tiny-dash/api:<X.Y.Z> (+ latest, sha-…)
 │
 ▼
Deploy (manual)   helm upgrade --install …          # build & push only; you roll out
```

Versioning is driven by [Conventional Commits](https://www.conventionalcommits.org/) via
`nx release` (configured in `nx.json`). `fix:` → patch, `feat:` → minor, `feat!:` /
`BREAKING CHANGE:` → major. Only commits that affect the `api` project (or shared root
files) bump it — firmware-only commits (`inky`, `picoDevice`) are ignored, and automated
release commits are filtered out of the changelog.

## Running the image

```bash
docker run --rm -p 8080:8080 \
  -e TINYDASH_HA_BASE_URL="http://homeassistant.local:8123" \
  -e TINYDASH_HA_TOKEN="<long-lived-token>" \
  ghcr.io/thiemok/tiny-dash/api:<version>

curl -fs localhost:8080/api/hello                 # -> Hello, World!
# mock render (no Home Assistant needed):
curl -fs "localhost:8080/api/dashboard/preview?width=800&height=480&colors=0,1,2,3&mock=1" -o out.png
```

`CHROME_PATH` is baked into the image (`/headless-shell/headless-shell`); you only override
it if you build a custom image.

## Deploying with Helm

The chart lives in `charts/api`. Its `appVersion` tracks the released image, so the image
tag defaults to the chart's `appVersion` unless you override `image.tag`.

```bash
helm upgrade --install tiny-dash-api ./charts/api \
  --namespace tiny-dash --create-namespace \
  --set secret.token="<home-assistant-long-lived-token>" \
  --set config.haBaseUrl="http://homeassistant.local:8123"
# pin a specific image: --set image.tag=1.4.0
```

Manage the Home Assistant token yourself instead of via the chart:

```bash
kubectl -n tiny-dash create secret generic ha-token \
  --from-literal=TINYDASH_HA_TOKEN="<token>"

helm upgrade --install tiny-dash-api ./charts/api -n tiny-dash \
  --set secret.existingSecret=ha-token
```

### Key chart values

| Value | Default | Purpose |
|---|---|---|
| `image.repository` | `ghcr.io/thiemok/tiny-dash/api` | Image to deploy |
| `image.tag` | `""` (→ chart `appVersion`) | Pin a specific version |
| `config.haBaseUrl` | `http://homeassistant.local:8123` | `TINYDASH_HA_BASE_URL` (ConfigMap) |
| `config.weatherEntity` | `weather.home` | `TINYDASH_WEATHER_ENTITY` (ConfigMap) |
| `config.extraEnv` | `{}` | Extra `TINYDASH_*` env (ConfigMap) |
| `secret.token` | `""` | HA token; chart-managed Secret |
| `secret.existingSecret` | `""` | Use a Secret you manage (key `TINYDASH_HA_TOKEN`) |
| `resources.limits.memory` | `1Gi` | Chrome needs headroom while rendering |
| `dshm.enabled` | `true` | Larger `/dev/shm` for Chrome |
| `ingress.enabled` | `false` | Expose via Ingress |

Probes hit `GET /api/hello`. Verify a deploy:

```bash
kubectl -n tiny-dash port-forward svc/tiny-dash-api 8080:80
curl -fs localhost:8080/api/hello
```

## Notes & follow-ups

- **CI scope:** `ci.yml` excludes the TinyGo firmware projects (`inky`, `picoDevice`) from
  the build, since they need the TinyGo toolchain. A dedicated TinyGo job can cover them
  later.
- **Image size:** ~600 MB (headless Chrome + fonts). Fonts (`fonts-liberation`,
  `fonts-noto-*`) are required — without them dashboard text renders as tofu boxes.
- **Loop safety:** the release bot's commits carry `[skip ci]`, so its pushes (via PAT,
  which do retrigger workflows) don't loop.
