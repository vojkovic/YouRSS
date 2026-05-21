# YouRSS

A minimal, no frills RSS feed reader for YouTube.

![YouRSS interface](docs/Screenshot.png)

The header shows fetch health (`3/3 channels · 39 videos · updated 2 minutes ago`) and keeps cached results when a channel fails to refresh.

## Configuration

Create a `config.yaml` in the working directory:

```yaml
channels:
  Computerphile: "UC9-y-6csu5WGm29I7JiwpnA"
  Fireship: "UCsBjURrPoezykLs9EqgamOA"
  TomScott: "UCBa659QWEk1AI4Tg--mrJ2A"
```

The key is a label for your own reference (used in logs and error messages). The value is the YouTube channel ID from the channel URL.

Use the converter at the bottom of the page to turn a channel link like `https://www.youtube.com/@Computerphile` into a config line you can paste into `config.yaml`.

## Run locally

Requires Go 1.23+.

```bash
go run main.go
```

Or:

```bash
make
```

Open http://localhost:8080. Feeds refresh every 5 minutes by default.

## Environment variables

- `PORT` - HTTP port to listen on. Defaults to `8080`.
- `REFRESH_INTERVAL` - How often to refetch feeds. Defaults to `5m`. Accepts Go duration strings like `10m`, `1h`, `30s`.
- `VIDEO_URL` - Base URL for video links. Leave unset to use YouTube. Set to an Invidious or Piped instance to rewrite links, e.g. `https://invidious.tiekoetter.com`.

```bash
PORT=3000 VIDEO_URL=https://invidious.tiekoetter.com go run main.go
```

## Docker

Prebuilt image from GitHub Container Registry:

```bash
docker run -d -p 8080:8080 \
  -v /path/to/config.yaml:/config.yaml \
  -e VIDEO_URL=https://invidious.tiekoetter.com \
  ghcr.io/vojkovic/yourss
```

Build and run locally:

```bash
make docker
```

### Docker Compose

```yaml
services:
  yourss:
    image: ghcr.io/vojkovic/yourss
    restart: always
    ports:
      - "8080:8080"
    volumes:
      - /path/to/config.yaml:/config.yaml
```

```bash
docker compose up -d
```
