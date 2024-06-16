# Peer Tester

Test your DN42 peers by sending packets with the random source IPs ``172.20.0.53`` / ``fd42:d42:d42:54::1`` and your host's IP as destination IP.
Works on layer 3 (tun) interfaces. The application automatically sends test packets on all interfaces.

This will test:
- Forwarding setup of your peer
- Peer's installation of routes received via BGP (if the destination addresses used are announced via BGP)
- Misconfigurations of your peer such as NAT
- State of the tunnel

````
Usage of ./peertester:
  -dst4 string
    	destination IPv4 address (the address this host can be reached from) or CIDR to find address from 'lo'
  -dst6 string
    	destination IPv6 address (the address this host can be reached from) or CIDR to find address from 'lo'
  -interface string
    	optional comma-separated target interface(s). Use '-' to read from stdin 
  -json
    	output as JSON
````