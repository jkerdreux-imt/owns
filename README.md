# OwNS

OwNS (Own Name Server) is a personal DNS server designed to solve issues
related to VPN usage, especially accessing DNS servers within private networks.
It combines flexible configuration per domain or network, multi-server
management, and a simple static hosts file.

---

## Table of Contents
- [Features](#features)
- [Installation](#installation)
- [Configuration](#configuration)
  - [forward.yaml](#forwardyaml)
  - [hosts.txt](#hoststxt)
- [Build & Binaries](#build--binaries)
- [Usage](#usage)
  - [Command Line Flags](#command-line-flags)
  - [Systemd Integration](#systemd-integration)
- [Dependencies](#dependencies)
- [Contributing](#contributing)
- [Support](#support)
- [License](#license)

---

## Features
- **Recursion & cache** (like dnsmasq)
- **Custom DNS servers** per domain or network slice
- **Static hosts file** (dnsmasq-style format)
- **UDP, TCP, TLS (DoT) support**
- **Flexible configuration via YAML and hosts.txt**

---

## Installation

### Prerequisites
- Go >= 1.18
- make

### From source
```shell
git clone https://github.com/jkerdreux-imt/owns.git
cd owns
make
sudo make install
```

### Binaries
Precompiled binaries for various platforms (Linux, Darwin, NetBSD, Windows,
ARM64) are available in the [GitHub Releases](https://github.com/jkerdreux-imt/owns/releases).

---

## Configuration

Default configuration files are located in `/etc/owns/`:
- `forward.yaml`: DNS server configuration per domain/network
- `hosts.txt`: Static entries (dnsmasq format)

### forward.yaml

The `forward.yaml` file lets you define which DNS servers to use for each
network or domain. Here is a sample configuration with multiple entries:

```yaml
- networks:
    - 192.168.1.0/24
    - 2001:555:4444:3333::/64
  domains:
    - home
  servers:
    - udp://[2001:555:4444:3333::254]
    - tls://192.168.1.254

- networks:
    - 10.0.0.0/8
    - 192.44.75.0/24
  domains:
    - imt-atlantique.fr
  servers:
    - udp://192.44.75.10

- servers:
    - udp://8.8.8.8
    - tls://[2620:fe::9]
```

#### Part 1: Home network and domain
This configuration will use the listed servers for any domain ending in `.home`
or any IP in the specified networks.

#### Part 2: Organization domain and network
This configuration will use the listed server for any domain ending in
`.imt-atlantique.fr` or any IP in the specified networks.

#### Part 3: Default servers
```yaml
- servers:
    - udp://8.8.8.8
    - tls://[2620:fe::9]
```
This block defines default servers used for queries that do not match any
specific network or domain above.

- Networks and domains can overlap: the first match is used.
- Default servers are those without associated domains/networks.
- Supported schemes: `udp://`, `tcp://`, `tls://` (DoT).

### hosts.txt
Static entries:
```
test0.home,192.168.1.2,2001:666:5555:4444::2,test 00 VM
test1.home,192.168.1.3,2001:666:5555:4444::3,test 01 VM
test2.home,192.168.1.4,,test 02 VM
```
- The ipv6 and txt fields are optional.

---

## Build & Binaries

To build manually from source:
```shell
make
```
This will generate binaries in the `bin/` directory for development or custom builds.
For official releases and precompiled binaries, visit the [GitHub Releases](https://github.com/jkerdreux-imt/owns/releases) page.

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

## Dependencies

OwNS uses the following Go modules:

- [github.com/miekg/dns](https://github.com/miekg/dns): DNS protocol implementation for Go
- [github.com/sirupsen/logrus](https://github.com/sirupsen/logrus): Structured logging for Go
- [gopkg.in/yaml.v3](https://github.com/go-yaml/yaml): YAML parsing and encoding

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
