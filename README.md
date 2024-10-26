# YouRSS

YouRSS is a simple RSS feed reader for YouTube channels. It is a web application that allows you to subscribe to YouTube channels and receive their latest videos in your feed. Simply provide a list of YouTube channel IDs and YouRSS will take care of the rest.


## Running YouRSS

First create a config.yaml file with a list of YouTube channels you want to subscribe to. The file should look like this:

```yaml
channels:
  Computerphile: "UC9-y-6csu5WGm29I7JiwpnA"
  Fireship: "UCsBjURrPoezykLs9EqgamOA"
  TomScott: "UCBa659QWEk1AI4Tg--mrJ2A"
```

The name of the channel is the key and the channel ID is the value. The name can be anything you want, it is just so you can identify the channel in the configuration file. The channel ID is the unique identifier for the channel and can be found in the URL of the channel page.

YouRSS is a Go application and can be run as a standalone binary. It is also available as a <9MB Docker image which is the recommended way to run it.

### Docker

YouRSS is available as a Docker image on GitHub Container Registry. You can run it with the following command:

```bash
docker run -d -p 8080:8080 -v /path/to/config.yaml:/config.yaml ghcr.io/vojkovic/yourss
```

### Docker Compose

You can also use Docker Compose to run YouRSS. Create a `docker-compose.yaml` file with the following content:

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

Then run the following command:

```bash
docker-compose up -d
```

