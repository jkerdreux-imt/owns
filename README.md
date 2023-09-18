# OwNS

OwNS is a personal DNS server. It is designed to solve the problems associated with VPN use, and in particular access to the DNS servers used in the private network.

Owns provides the following features: 
- recursion & cache (like dnsmasq)
- use of specific servers per domain or network slice. 
- use of a hosts file containing static entries (dnsmasq-style format).


## Installation
```shell
git clone https://github.com/jkerdreux-imt/owns.git
cd owns
make
sudo make install
```

## Configuration

By default, OwNS uses two configuration files located in /etc/owns/

## forward.yaml
The `forward.yaml` file contains the list of different DNS servers to be used. The servers field contains the list of servers used for this zone. These servers are used according to the domain (direct lookup), or the associated networks (reverse lookup). Example : 


```yaml
  - networks:
        - 192.168.1.0/24
        - 2001:555:4444:3333::/64
    domains:
        - home
    servers:
        - 2001:555:4444:3333::254
        - 192.168.1.254

  - networks:
        - 10.0.0.0/8
        - 192.44.75.0/24
    domains:
        - imt-atlantique.fr
    servers:
        - 192.44.75.10
```

If we search for any domain name ending in `.home`, we will only use servers `192.168.1.254` or `2001:555:4444:3333::254`. Same for reverse lookup: We will use the same servers if you query any IP within networks `192.168.1.0/24` or `2001:555:4444:3333::/64` (cidr notation).

Important:
- You can use some overlapping in networks, the first one will be used for reverse lookups.
- Entries with no domains and networks are considered as default servers.


## hosts.txt
The `hosts.txt` file contains the list of static entries (dnsmasq style), with the following fields: fqdn,ipv4,ipv6, txt 

```
test0.home,192.168.1.2,2001:666:5555:4444::2,test 00 VM
test1.home,192.168.1.3,2001:666:5555:4444::3,test 01 VM
test2.home,192.168.1.4,,test 02 VM
```

The ipv6 and txt fields are optional. 


## Integration
OwNS provide a `systemd` unit file and you can enable it as usual with systemd.
```shell
 sudo systemctl [start|stop|enable|disable|status] owns
```


## Command line flags
```shell
Usage of owns:
  -bindAddr string
        Address to which the server should bind (default "0.0.0.0")
  -confDir string
        Configuration directory (default "/etc/owns")
  -logLevel string
        Log level (e.g., INFO, DEBUG) (default "INFO")
  -port int
        Port on which the server should listen (default 53)
```


 ## TODO
   - Add support for DoT or DoH. 

## Additionnal notes
- This is my first real program writen in Golang, so it may contains some errors, but I use it as my daily NS for quite a long time now.
- There is no default zone associated with hosts file. So, if you query a local host which is not in the hosts file, OwNS will forward the query to the default servers (DNS leak..)
