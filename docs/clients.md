# Configuring clients to use Wormhole

Point any HTTP client at Wormhole with a single proxy address:
`http://HOST:8080` (or `http://user:pass@HOST:8080` if Basic auth is enabled).
The **same address handles both HTTP and HTTPS** — HTTPS is tunnelled through
`CONNECT`, so you do *not* need a separate "secure proxy" entry.

Replace `HOST` with the machine running Wormhole (`localhost` if it's the same
machine) throughout.

---

## Table of contents

- [Quick per-app testing (curl)](#quick-per-app-testing-curl)
- [Linux](#linux)
- [macOS](#macos)
- [Windows](#windows)
- [Browsers](#browsers)
- [Verifying it works](#verifying-it-works)
- [Turning it off](#turning-it-off)

---

## Quick per-app testing (curl)

```sh
curl -x http://HOST:8080 https://example.com
# with auth:
curl -x http://alice:s3cret@HOST:8080 https://example.com
```

---

## Linux

### Per-shell environment variables

Most command-line tools (`curl`, `wget`, `apt`, `git`, most language runtimes)
honour the standard proxy environment variables. Set them in your shell:

```sh
export http_proxy=http://HOST:8080
export https_proxy=http://HOST:8080
# With auth:
export http_proxy=http://alice:s3cret@HOST:8080
export https_proxy=http://alice:s3cret@HOST:8080
# Don't proxy local traffic:
export no_proxy=localhost,127.0.0.1,::1
```

Some tools only read the uppercase forms — export both to be safe:

```sh
export HTTP_PROXY=$http_proxy HTTPS_PROXY=$https_proxy NO_PROXY=$no_proxy
```

Make it permanent by adding those lines to `~/.bashrc`, `~/.zshrc`, or
`/etc/environment` (system-wide).

### GNOME desktop (system-wide)

```sh
gsettings set org.gnome.system.proxy mode 'manual'
gsettings set org.gnome.system.proxy.http  host 'HOST'
gsettings set org.gnome.system.proxy.http  port 8080
gsettings set org.gnome.system.proxy.https host 'HOST'
gsettings set org.gnome.system.proxy.https port 8080
# Back to direct:
gsettings set org.gnome.system.proxy mode 'none'
```

Or use **Settings → Network → Network Proxy → Manual** and enter `HOST` / `8080`
for both HTTP and HTTPS.

### apt (Debian/Ubuntu)

```sh
echo 'Acquire::http::Proxy  "http://HOST:8080";'  | sudo tee /etc/apt/apt.conf.d/00proxy
echo 'Acquire::https::Proxy "http://HOST:8080";' | sudo tee -a /etc/apt/apt.conf.d/00proxy
```

---

## macOS

### System-wide (GUI)

**System Settings → Network → (your service, e.g. Wi-Fi) → Details… → Proxies**

1. Enable **Web Proxy (HTTP)**, enter server `HOST`, port `8080`.
2. Enable **Secure Web Proxy (HTTPS)**, enter server `HOST`, port `8080`.
3. If auth is enabled, tick **Proxy server requires password** and fill in the
   username/password.
4. Click **OK → Apply**.

### System-wide (command line)

Find your service name with `networksetup -listallnetworkservices`, then:

```sh
networksetup -setwebproxy        "Wi-Fi" HOST 8080
networksetup -setsecurewebproxy  "Wi-Fi" HOST 8080
# With auth (append: on <user> <pass>):
networksetup -setwebproxy        "Wi-Fi" HOST 8080 on alice s3cret
networksetup -setsecurewebproxy  "Wi-Fi" HOST 8080 on alice s3cret

# Turn off:
networksetup -setwebproxystate       "Wi-Fi" off
networksetup -setsecurewebproxystate "Wi-Fi" off
```

### Shell tools

The Linux environment-variable approach works identically in Terminal on macOS:

```sh
export https_proxy=http://HOST:8080 http_proxy=http://HOST:8080
```

---

## Windows

### System-wide (Settings app)

**Settings → Network & Internet → Proxy → Manual proxy setup → Set up**

- Address: `HOST`  Port: `8080`  → **Save**

This applies to Edge, Chrome, and most Windows apps.

### System-wide (PowerShell / netsh)

```powershell
# Uses the same host:port for HTTP and HTTPS.
netsh winhttp set proxy HOST:8080

# Inspect / reset:
netsh winhttp show proxy
netsh winhttp reset proxy
```

> `netsh winhttp` configures the WinHTTP stack used by many services and
> background components. Browsers typically follow the **Settings → Proxy**
> (WinINET) configuration above.

### PowerShell session only

```powershell
$env:HTTP_PROXY  = "http://HOST:8080"
$env:HTTPS_PROXY = "http://HOST:8080"
# With auth:
$env:HTTPS_PROXY = "http://alice:s3cret@HOST:8080"
```

### Command Prompt session only

```bat
set HTTP_PROXY=http://HOST:8080
set HTTPS_PROXY=http://HOST:8080
```

---

## Browsers

- **Chrome / Edge** — follow the OS system settings above (they use the system
  proxy by default), or launch with a flag for an isolated setup:
  ```sh
  google-chrome --proxy-server="http://HOST:8080"
  ```
- **Firefox** has its own setting: **Settings → Network Settings → Manual proxy
  configuration**. Set **HTTP Proxy** to `HOST` / `8080` and tick
  **Also use this proxy for HTTPS**.

---

## Verifying it works

1. Start Wormhole with `--verbose` so requests are logged.
2. Configure a client as above and make a request.
3. You should see a matching `http` or `connect` log line from Wormhole, e.g.:
   ```
   level=INFO msg=connect host=example.com:443 client=192.0.2.10 duration=41ms
   ```
4. From the shell you can confirm the exit IP with:
   ```sh
   curl -x http://HOST:8080 https://api.ipify.org ; echo
   ```

---

## Turning it off

- **Env vars:** `unset http_proxy https_proxy HTTP_PROXY HTTPS_PROXY`
- **macOS:** `networksetup -setwebproxystate "Wi-Fi" off` (and `...securewebproxystate`)
- **Windows:** `netsh winhttp reset proxy`, and toggle off under Settings → Proxy
- **GNOME:** `gsettings set org.gnome.system.proxy mode 'none'`
