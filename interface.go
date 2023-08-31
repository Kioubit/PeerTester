package main

import (
	"errors"
	"fmt"
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
	"net"
	"net/netip"
	"syscall"
)

func open(ifName string, v6 bool) (int, error) { //(net.PacketConn, error) {
	var addressFamily = syscall.AF_INET
	if v6 {
		addressFamily = syscall.AF_INET6
	}

	var sockOptIntValue = syscall.IPPROTO_IP
	if v6 {
		sockOptIntValue = syscall.IPPROTO_IPV6
	}

	fd, err := syscall.Socket(addressFamily, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return -1, fmt.Errorf("failed open socket(syscall.AF_INET{6}, syscall.SOCK_RAW, syscall.IPPROTO_RAW): %s", err)
	}
	err = syscall.SetsockoptInt(fd, sockOptIntValue, syscall.IP_HDRINCL, 1)
	if err != nil {
		return -1, errors.New("could not set socket options")
	}

	if ifName != "" {
		_, err := net.InterfaceByName(ifName)
		if err != nil {
			return -1, fmt.Errorf("failed to find interface: %s: %s", ifName, err)
		}
		err = syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, ifName)
		if err != nil {
			return -1, errors.New("could not set bind socket options")
		}
	}
	return fd, nil
}

func sendOnInterface(intFace net.Interface, dstIP net.IP, dstIP6 net.IP) {
	fmt.Printf("[%s] ", intFace.Name)
	dst := &net.UDPAddr{
		IP:   dstIP,
		Port: 5000,
	}
	src := &net.UDPAddr{
		IP:   net.ParseIP("172.20.0.53"),
		Port: 5000,
	}
	dst6 := &net.UDPAddr{
		IP:   dstIP6,
		Port: 5000,
	}
	src6 := &net.UDPAddr{
		IP:   net.ParseIP("fd42:d42:d42:54::1"),
		Port: 5000,
	}

	hmacKeyLock.Lock()
	toSend := hmacSeal([16]byte(hmacKey), []byte(intFace.Name))
	hmacKeyLock.Unlock()

	sendOnInterfaceIPv4(intFace, dst, src, toSend)
	sendOnInterfaceIPv6(intFace, dst6, src6, toSend)

}

func sendOnInterfaceIPv4(intFace net.Interface, dst *net.UDPAddr, src *net.UDPAddr, toSend []byte) {
	conn, err := open(intFace.Name, false)
	if err != nil {
		fmt.Printf(" -- [%s] Error: %s", intFace.Name, err)
		return
	}
	defer func(fd int) {
		_ = syscall.Close(fd)
	}(conn)
	b, err := buildUDPPacket(dst, src, toSend)
	if err != nil {
		fmt.Printf(" --  [%s] Error: %s", intFace.Name, err)
		return
	}
	err = syscall.Sendto(conn, b, 0, &syscall.SockaddrInet4{
		Port: 5000,
		Addr: [4]byte(dst.IP.To4()),
	})
	if err != nil {
		fmt.Printf(" --  [%s] Error: %s", intFace.Name, err)
		return
	}
}

func sendOnInterfaceIPv6(intFace net.Interface, dst6 *net.UDPAddr, src6 *net.UDPAddr, toSend []byte) {
	conn, err := open(intFace.Name, true)
	if err != nil {
		fmt.Printf(" -- [%s] Error: %s", intFace.Name, err)
		return
	}
	defer func(fd int) {
		_ = syscall.Close(fd)
	}(conn)

	b, err := buildUDPPacket6(dst6, src6, toSend)
	if err != nil {
		fmt.Printf(" -- [%s] Error: %s", intFace.Name, err)
		return
	}

	isPointToPoint, err := isInterfacePointToPoint(int32(intFace.Index))
	if err != nil {
		fmt.Printf(" -- [%s] Error: %s", intFace.Name, err)
		return
	}

	if !isPointToPoint {
		fmt.Print(" Interface is not point to point ")
		targetLinkLocal, _ := netip.AddrFromSlice(dst6.IP)
		err = syscall.Sendto(conn, b, 0, &syscall.SockaddrInet6{
			ZoneId: 0,
			Port:   0,
			Addr:   targetLinkLocal.As16(),
		})
		if err != nil {
			fmt.Printf(" --  [%s] Error: %s", intFace.Name, err)
			return
		}
		return
	}
	hasLL, err := hasInterfaceLinkLocal(intFace)
	if err != nil {
		fmt.Printf(" --  [%s] Error: %s", intFace.Name, err)
		return
	}

	if hasLL {
		fmt.Print(" interface has link-local ")
		targetLinkLocal, err := netip.ParseAddr("fe80::1e6f:84f7:7ba0:945d")
		err = syscall.Sendto(conn, b, 0, &syscall.SockaddrInet6{
			ZoneId: 0,
			Port:   0,
			Addr:   targetLinkLocal.As16(),
		})
		if err != nil {
			fmt.Printf(" --  [%s] Error (Link-local): %s", intFace.Name, err)
			return
		}
		return
	}

	peerIPs, err := getPeerIP(syscall.AF_INET6, int32(intFace.Index))
	if err != nil {
		fmt.Printf(" --  [%s] Error: %s", intFace.Name, err)
		return
	}

	if len(peerIPs) == 0 {
		fmt.Printf(" --  [%s] Error: No peer IPs", intFace.Name)
		return
	}

	targetPeerIP, err := netip.ParseAddr(peerIPs[0].String())
	fmt.Print(" Found peerIP ", targetPeerIP.String())
	err = syscall.Sendto(conn, b, 0, &syscall.SockaddrInet6{
		ZoneId: 0,
		Port:   0,
		Addr:   targetPeerIP.As16(),
	})
	if err != nil {
		fmt.Printf(" --  [%s] Error (ULA): %s -> Trying to obtain a peer address", intFace.Name, err)
		return
	}

}

func hasInterfaceLinkLocal(intFace net.Interface) (bool, error) {
	intAddresses, err := intFace.Addrs()
	if err != nil {
		return false, err
	}
	ll, _ := netip.ParsePrefix("fe80::/10")
	for _, address := range intAddresses {
		prefix, err := netip.ParsePrefix(address.String())
		if err != nil {
			return false, err
		}
		if ll.Contains(prefix.Addr()) {
			return true, nil
		}
	}
	return false, nil
}

func getPeerIP(family int, interfaceIndex int32) ([]net.IP, error) {
	req := nl.NewNetlinkRequest(syscall.RTM_GETADDR, syscall.NLM_F_DUMP|syscall.NLM_F_REQUEST)
	infoMsg := &nl.IfAddrmsg{
		IfAddrmsg: unix.IfAddrmsg{
			Family: uint8(family),
			Index:  uint32(interfaceIndex),
		}}
	req.AddData(infoMsg)

	messages, err := req.Execute(syscall.NETLINK_ROUTE, syscall.RTM_NEWADDR)
	if err != nil {
		return nil, err
	}

	peerIPs := make([]net.IP, 0)
	localIPs := make([]net.IP, 0)

	for _, m := range messages {
		msg := nl.DeserializeIfAddrmsg(m)
		if msg.Index != uint32(interfaceIndex) {
			continue
		}
		attrs, err := nl.ParseRouteAttr(m[msg.Len():])
		if err != nil {
			return nil, err
		}
		var ip net.IP
		for _, attr := range attrs {
			switch attr.Attr.Type {
			// IFA_ADDRESS is the peer address in point to point links, IFA_LOCAL is the local ip
			case syscall.IFA_LOCAL:
				ip = attr.Value
				localIPs = append(localIPs, ip)
			case syscall.IFA_ADDRESS:
				ip = attr.Value
				peerIPs = append(peerIPs, ip)
			}
		}
	}

	result := make([]net.IP, 0)

	for _, pIP := range peerIPs {
		found := false
		for _, lIP := range localIPs {
			if lIP.Equal(pIP) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, pIP)
		}
	}
	return result, nil
}

func isInterfacePointToPoint(interfaceIndex int32) (bool, error) {
	req := nl.NewNetlinkRequest(syscall.RTM_GETLINK, syscall.NLM_F_REQUEST)
	infoMsg := &nl.IfInfomsg{
		IfInfomsg: unix.IfInfomsg{
			Family: syscall.AF_UNSPEC,
			Index:  interfaceIndex,
		}}
	req.AddData(infoMsg)
	messages, err := req.Execute(syscall.NETLINK_ROUTE, syscall.RTM_NEWLINK)
	if err != nil {
		return false, err
	}
	for _, m := range messages {
		msg := nl.DeserializeIfInfomsg(m)
		return msg.Flags&syscall.IFF_POINTOPOINT != 0x0, nil
	}
	return false, nil
}
