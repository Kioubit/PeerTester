# PeerTester

Test your DN42 peers by sending packets with the random source IPs ``172.20.0.53`` / ``fd42:d42:d42:54::1`` and your host's IP as destination IP.
Works on layer 3 (tun) interfaces. The application automatically sends test packets on all interfaces by default.

This will test:
- Forwarding setup of your peer
- Peer's installation of routes received via BGP
- Misconfigurations of your peer such as NAT
- Unexpected forwarding paths (via TTL measurements)
- State and latency of the tunnel

## Important setup notes
- The `dst4` and `dst6` IP addresses should be announced via BGP and *not* be IP addresses used for the peer tunnels.
- If the status of a peering is not shown as `OK` for either IPv4 or IPv6, then the latency values returned are invalid and informational only.

## Usage
````
Usage of ./peertester:
  -daemon
        run as a daemon and accept interface lists via unix socket
  -dst4 string
        destination IPv4 address (the address this host can be reached from) or CIDR to find address from 'lo'
  -dst6 string
        destination IPv6 address (the address this host can be reached from) or CIDR to find address from 'lo'
  -interface string
        optional comma-separated target interface(s). Use '-' to read from stdin. If not specified, packets are sent on all interfaces
  -json
        output as JSON
````
