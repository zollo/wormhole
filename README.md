# 🌀 Wormhole

A tiny, dependency-free **forward HTTP/HTTPS proxy** built for one thing: being
quick and easy to deploy.

- **Single static binary** — no runtime, no libraries. Built with the Go
  standard library only.
- **HTTP and HTTPS** — forwards plain HTTP and tunnels HTTPS (and anything
  else) via `CONNECT`.
- **Configure with flags or environment variables** — drop it into a container
  or a systemd unit with zero fuss.
- **Optional Basic auth**, graceful shutdown, structured logs, sane timeouts.

---

## Quick start

```sh
# Run an open proxy on :8080
wormhole

# Point a client at it
curl -x http://localhost:8080 https://example.com
export https_proxy=http://localhost:8080
```

With authentication:

```sh
wormhole --user alice --pass s3cret
curl -x http://alice:s3cret@localhost:8080 https://example.com
```

---

## Install

### Download a binary

Grab the archive for your platform from the
[Releases page](https://github.com/zollo/wormhole/releases), extract it, and put
`wormhole` on your `PATH`:

```sh
tar -xzf wormhole_*_linux_amd64.tar.gz
sudo install wormhole /usr/local/bin/
```

Binaries are published for Linux, macOS, and Windows on amd64 and arm64.

### Docker

```sh
docker run -p 8080:8080 ghcr.io/zollo/wormhole:latest
```

### Go install

```sh
go install github.com/zollo/wormhole@latest
```

### Build from source

```sh
git clone https://github.com/zollo/wormhole
cd wormhole
make build      # produces ./wormhole
```

---

## Configuration

Every option can be set with a flag **or** a `WORMHOLE_*` environment variable.
Flags take precedence.

| Flag             | Env var                  | Default | Description                                    |
| ---------------- | ------------------------ | ------- | ---------------------------------------------- |
| `--addr`         | `WORMHOLE_ADDR`          | `:8080` | Address to listen on (`host:port`).            |
| `--user`         | `WORMHOLE_USER`          | —       | Basic-auth username (empty = open proxy).      |
| `--pass`         | `WORMHOLE_PASS`          | —       | Basic-auth password.                           |
| `--dial-timeout` | `WORMHOLE_DIAL_TIMEOUT`  | `10s`   | Timeout for connecting to origin servers.      |
| `--read-timeout` | `WORMHOLE_READ_TIMEOUT`  | `30s`   | Timeout for reading a client request.          |
| `--idle-timeout` | `WORMHOLE_IDLE_TIMEOUT`  | `120s`  | Keep-alive timeout for idle client sockets.    |
| `--verbose`      | `WORMHOLE_VERBOSE`       | `false` | Enable debug logging.                          |
| `--version`      | —                        | —       | Print version and exit.                        |

> **Security note:** an open proxy will relay traffic for anyone who can reach
> it. Bind it to a private interface, put it behind a firewall, or enable Basic
> auth before exposing it.

---

## Deploy as a service

### systemd (binary)

Install the binary and the provided unit:

```sh
sudo install wormhole /usr/local/bin/
sudo useradd --system --no-create-home --shell /usr/sbin/nologin wormhole
sudo cp deploy/wormhole.service /etc/systemd/system/
sudo cp deploy/wormhole.env.example /etc/wormhole.env   # then edit
sudo systemctl daemon-reload
sudo systemctl enable --now wormhole
```

Configuration is read from `/etc/wormhole.env`. The unit is hardened
(`NoNewPrivileges`, `ProtectSystem=strict`, dropped capabilities, etc.) and
restarts on failure. See [`deploy/wormhole.service`](deploy/wormhole.service).

### Docker Compose

```sh
cd deploy
docker compose up -d
```

See [`deploy/docker-compose.yml`](deploy/docker-compose.yml). It uses
`restart: unless-stopped`, a read-only root filesystem, and
`no-new-privileges`.

### Docker under systemd

Prefer the container image but want systemd to supervise it? Use
[`deploy/wormhole-docker.service`](deploy/wormhole-docker.service), which pulls
the image and runs it with `--env-file /etc/wormhole.env`.

### Plain `docker run` (as a restarting service)

```sh
docker run -d --name wormhole \
  --restart unless-stopped \
  -p 8080:8080 \
  -e WORMHOLE_USER=alice -e WORMHOLE_PASS=s3cret \
  --read-only --security-opt no-new-privileges:true \
  ghcr.io/zollo/wormhole:latest
```

---

## Configure your clients

Point any HTTP client at `http://HOST:8080` — the same address serves both HTTP
and HTTPS. See **[docs/clients.md](docs/clients.md)** for step-by-step setup on
**Linux**, **macOS**, and **Windows** (environment variables, system-wide
settings, and browsers), plus how to verify and disable it.

```sh
export https_proxy=http://HOST:8080   # Linux/macOS shell
```

---

## How it works

- **HTTP requests** arrive with an absolute URL. Wormhole strips hop-by-hop
  headers, appends `X-Forwarded-For`, forwards the request to the origin, and
  streams the response back.
- **HTTPS / TLS** arrives as a `CONNECT host:443` request. Wormhole opens a TCP
  connection to the target, replies `200 Connection Established`, and then
  blindly pipes bytes in both directions — it never sees the encrypted payload.

---

## Development

```sh
make test    # go test -race ./...
make vet
make build
make docker
```

Releases are cut by pushing a `vX.Y.Z` tag; a GitHub Actions workflow runs
[GoReleaser](https://goreleaser.com) to build the binaries and publish the
container image to GHCR.

---

## License

[MIT](LICENSE)
