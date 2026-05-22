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

Open http://localhost:8080. Feeds refresh every 15 minutes by default.

YouTube's public Atom feed (`/feeds/videos.xml`) sometimes returns 404 for every channel during certain hours. YouRSS keeps the last successful fetch until the endpoint works again.

## Environment variables

- `PORT` - HTTP port to listen on. Defaults to `8080`.
- `REFRESH_INTERVAL` - How often to refetch feeds. Defaults to `15m`. Accepts Go duration strings like `5m`, `30m`, `1h`. With many channels (20+), consider `30m` to reduce YouTube rate limiting.
- `HTTP_TIMEOUT` - Per-request timeout for YouTube and thumbnail fetches. Defaults to `30s`.
- `CHANNEL_FETCH_DELAY` - Pause between channel fetches during a refresh. Defaults to `2s` when you have more than 10 channels, otherwise `500ms`.
- `VIDEO_URL` - Base URL for video links. Leave unset to use YouTube. Set to an Invidious or Piped instance to rewrite links, e.g. `https://invidious.tiekoetter.com`.
- `HIDE_SHORTS` - Exclude YouTube Shorts from the feed. Defaults to `true`. Set to `false` to include Shorts.

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
