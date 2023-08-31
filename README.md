# Peer Tester
Test your DN42 peers by sending packets with the random source IPs ``172.20.0.53`` / ``fd42:d42:d42:54::1`` and your host's IP as destination IP.
The application automatically sends test packets on all interfaces.

This will test:
- Forwarding setup of your peer
- Misconfigurations of your peer such as NAT
- State of the tunnel