# modelsrv-git-sensor
GitOps-style sensor for EmELand YAML resources.

It scans a git repo for `*.yaml`/`*.yml` files in the EmELand format (`version: emeland.io/...`) and emits **Create** events for supported resource kinds. Events are forwarded to configured downstream model servers (subscribers).

## Run

```bash
go run ./cmd/modelsrv-git-sensor \
  --config config/sensor.yaml \
  --listen localhost:24100 \
  --poll-interval 10s
```

Config values live in [`config/sensor.yaml`](config/sensor.yaml):

- `repos[].branch`
- `repos[].checkoutDir`
- `repos[].paths`
- `repos[].repo`
- `subscribers`
- `watch`

## Subscriber management HTTP endpoints

The sensor exposes the same subscriber management endpoints as `modelsrv` (mounted under `/api`):

- `POST /api/events/register` (body: `{"callbackUrl":"http://downstream:24000/api/"}`) → **201**
- `POST /api/events/unregister` (body: `{"callbackUrl":"http://downstream:24000/api/"}`) → **200** or **404**
- `GET /api/events/subscribers` → `["http://downstream:24000/api/", ...]`
