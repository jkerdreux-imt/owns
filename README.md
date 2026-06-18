# OwNS

OwNS (Own Name Server) is a personal DNS server designed to solve issues
related to VPN usage, especially accessing DNS servers within private networks.
It combines flexible configuration per domain or network, multi-server
management, and a simple static hosts file.

---

## Table of Contents

- [Features](#features)
- [Configuration](#configuration)
  - [forward.yaml](#forwardyaml)
  - [hosts.txt](#hoststxt)
- [Usage](#usage)
  - [Command Line Flags](#command-line-flags)
  - [Systemd Integration](#systemd-integration)
- [Installation](#installation)
  - [Go](#go)
  - [From source](#from-source)
  - [Binaries](#binaries)
  - [AUR](#aur-arch-linux--manjaro)
  - [Docker](#docker)
- [Dependencies](#dependencies)
- [FAQ](#faq)
- [Contributing](#contributing)
- [Support](#support)
- [License](#license)

---

## Features

- **Recursion & cache** (like dnsmasq) — respects upstream TTL
- **Custom DNS servers** per domain or network slice
- **Static hosts file** (dnsmasq-style format)
- **UDP, TCP, TLS (DoT) support**
- **TCP/TLS connection pooling** (persistent connections per upstream server)
- **Flexible configuration via YAML and hosts.txt**

---

## Configuration

Default configuration files are located in `/etc/owns/`:

- `forward.yaml`: DNS server configuration per domain/network
- `hosts.txt`: Static entries (dnsmasq format)

### forward.yaml

The `forward.yaml` file lets you define which DNS servers to use for each
network or domain.

> 💡 A minimal setup with only default TLS servers acts as a simple DoT
> forwarder — like Stubby — encrypting all your DNS traffic.

Here is a sample configuration with multiple entries:

```yaml
# Vacation home — accessed through a VPN link
- networks:
    - 192.168.2.0/24
    - 2001:db8:2222::/48
  domains:
    - cottage
  servers:
    - udp://192.168.2.1
    - tls://[2001:db8:2222::1]

# Corporate network — broad range
- networks:
    - 10.0.0.0/8
  domains:
    - corporate.net
    - corporate.com
  servers:
    - udp://10.0.0.1

# Default servers — anything not matching the zones above
- servers:
    - tls://9.9.9.9
    - tls://[2620:fe::9]
```

#### Part 1: Remote home network and domain

This configuration will use the listed servers for any domain ending in `.cottage`
or any IP in the specified networks.

#### Part 2: Organization domain and network

This configuration will use the listed server for any domain ending in
`.corporate.net` or any IP in the specified networks.

#### Part 3: Default servers

```yaml
- servers:
    - tls://9.9.9.9
    - tls://[2620:fe::9]
```

This block defines default servers used for queries that do not match any
specific network or domain above.

- Networks and domains can overlap: the first match is used.
- Default servers are those without associated domains/networks.
- Supported schemes: `udp://`, `tcp://`, `tls://` (DoT).

#### DNSSEC
`DS` queries require a recursive resolver because the DS record lives in the
**parent zone** (e.g. `enstb.org DS` is in `.org`, not on `enstb.org`'s
authoritative server). When the zone server does not support recursion
(`ra=0`), OwNS automatically falls back to the default recursive servers for
that DS query only, preserving the response from the zone server in all other
cases.

#### TCP/TLS Connection Pool

OwNS maintains a pool of persistent connections to each upstream TCP/TLS server
(up to 4 per server). Connections are reused across queries to avoid the
overhead of repeated handshakes. When the pool is saturated, OwNS waits
briefly (100ms) then falls back to the next configured server. Broken
connections are automatically discarded and replaced on demand.

### hosts.txt

Static entries in `name,ipv4,ipv6,comment` format. ipv6 and comment are
optional:

```
test0.home,192.168.1.2,2001:666:5555:4444::2,test 00 VM
test1.home,192.168.1.3,2001:666:5555:4444::3,test 01 VM
test2.home,192.168.1.4,,test 02 VM
```

Hosts entries are served with a fixed TTL of 60 seconds.

---

## Usage

### Command Line Flags

```shell
owns -bindAddr "[::]" -confDir "/etc/owns" -logLevel "INFO" -port 53
```

**Available flags:**

- `-bindAddr`: Address to bind (default `[::]`)
- `-confDir`: Configuration directory (default `/etc/owns`)
- `-logLevel`: Log level (`INFO`, `DEBUG`, ...)
- `-port`: Listening port (default 53)

### Systemd Integration

A systemd service file is provided:

```shell
sudo systemctl [start|stop|enable|disable|status] owns
```

---

## Installation

### Go

```shell
go install github.com/jkerdreux-imt/owns@latest
```

Installs the latest version to `$GOPATH/bin`. Requires Go >= 1.18.

### From source

```shell
git clone https://github.com/jkerdreux-imt/owns.git
cd owns
make
sudo make install
```

**Prerequisites:** Go >= 1.18, make

### Binaries

Precompiled binaries for various platforms (Linux, Darwin, NetBSD, Windows,
ARM64) are available on the [GitHub Releases](https://github.com/jkerdreux-imt/owns/releases) page.

### AUR (Arch Linux / Manjaro)

```shell
yay -S owns
# or
pamac build owns
```

Then enable the service:

```shell
sudo systemctl enable --now owns
```

Configuration files are in `/etc/owns/`.

### Docker

**Using the pre-built image:**

```shell
docker pull ghcr.io/jkerdreux-imt/owns:latest
```

**Building the image:**

```shell
make docker-build
# or directly:
docker build -t owns .
```

**Running with docker-compose (recommended):**

The provided `docker-compose.yml` uses `network_mode: host` and mounts the
`./conf/` directory for live configuration:

```shell
docker compose up -d
docker compose logs -f
```

**Quick test with plain Docker:**

```shell
docker run --rm --network host -v ./conf/:/etc/owns/ \
  ghcr.io/jkerdreux-imt/owns:latest
```

Or build and run locally:

```shell
make docker-test
```

By default the container binds to `127.0.0.1:53` (UDP+TCP). Use custom flags to
override:

```shell
docker run --rm --network host -v ./conf/:/etc/owns/ \
  ghcr.io/jkerdreux-imt/owns:latest -logLevel DEBUG
```

---

## Dependencies

OwNS uses the following Go modules:

- [github.com/miekg/dns](https://github.com/miekg/dns): DNS protocol implementation for Go
- [github.com/sirupsen/logrus](https://github.com/sirupsen/logrus): Structured logging for Go
- [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml): YAML parsing and encoding

---

## FAQ

### Why not just use OpenVPN's `push dns`?

OpenVPN's `push "dhcp-option DNS"` replaces the entire DNS configuration.
With multiple VPNs the last one to connect wins - there is no native merge.
Reverse DNS (`dhcp-option DOMAIN`) has the same limitation: it works for
one VPN, not three simultaneously. `systemd-resolved` needed.

### Why not systemd-resolved?

`systemd-resolved` solves per-interface DNS and could handle multi-VPN setups -
if all VPN interfaces are local. This is not my usage. I run VPN clients
in containers, so systemd-resolved is not an option. In fact, systemd never will
be an option.
OwNS works regardless: it matches by IP prefix and uses the host routing table.

Also: no systemd, no dbus dependency.

### Why not dnsmasq or Unbound?

For _forward_ lookups, both work. Specific limitations:

- **dnsmasq** handles per-prefix reverse via `server=/10.0.0.0/8/...` but
  has no DoT support.
  With many corporate IP ranges and domains (20+ routing table entries for
  2 VPNs is typical), the flat config would be unreadable: every prefix and
  every domain needs its own line, with no grouping.

- **Unbound** works with DNS name trees (`.arpa`), not IP subnets. It
  requires octet-aligned reverse zones (`/8`, `/16`, `/24`). Arbitrary
  CIDR prefixes like `10.64.0.0/13` or `10.72.0.0/14` have no clean
  `.arpa` mapping and become unmanageable.

OwNS matches reverse lookups the same way as forward: by IP prefix -
any prefix - in the same YAML block. DoT is built-in. TCP/TLS connections
are pooled and reused across queries.

Additionally, when an upstream sets `RA=0` (authoritative-only), most
resolvers return SERVFAIL for DNSSEC DS queries.

### Comparison

|                           | dnsmasq | Unbound | systemd-resolved | OwNS |
|---------------------------|---------|---------|------------------|------|
| Forward per domain        | ✅      | ✅      | ✅               | ✅   |
| Reverse per prefix        | ✅      | ❌      | ✅               | ✅   |
| DoT upstream              | ❌      | ✅      | ✅               | ✅   |
| TCP/TLS pooling           | ❌      | ✅      | ✅               | ✅   |
| DS fallback (RA=0)        | ❌      | ✅      | ❌               | ✅   |
| Hosts file integrated     | ✅      | NA      | NA               | ✅   |
| No systemd required       | ✅      | ✅      | ❌               | ✅   |

---

## Contributing

Contributions are welcome! Please:

- Open an issue for suggestions or bugs
- Submit clear and documented pull requests
- Follow Go style (gofmt, staticcheck)

---

## Support

For questions or issues:

- Open a GitHub issue

---

## License

This project is licensed under the [BSD 3-Clause License](./LICENCE).

---

## Additional Notes

- OwNS has been used daily in my personal network configuration since 2023 without issues.
- Warning: There is no default zone associated with the hosts file. If you query
  a local host not in hosts.txt, OwNS will forward the query to the default
  servers (possible DNS leak).
