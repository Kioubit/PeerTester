package main

import (
	"errors"
	"fmt"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"net"
	"os"
	"syscall"
	"time"
)

func open(ifName string) (net.PacketConn, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		return nil, fmt.Errorf("failed open socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW): %s", err)
	}
	err = syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1)
	if err != nil {
		return nil, errors.New("could not set sockopt")
	}

	if ifName != "" {
		_, err := net.InterfaceByName(ifName)
		if err != nil {
			return nil, fmt.Errorf("failed to find interface: %s: %s", ifName, err)
		}
		err = syscall.SetsockoptString(fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, ifName)
		if err != nil {
			return nil, errors.New("could not set setsockoptBind")
		}
	}

	conn, err := net.FilePacketConn(os.NewFile(uintptr(fd), fmt.Sprintf("fd %d", fd)))
	if err != nil {
		return nil, err
	}
	return conn, nil
}

func buildUDPPacket(dst, src *net.UDPAddr, data []byte) ([]byte, error) {
	buffer := gopacket.NewSerializeBuffer()
	payload := gopacket.Payload(data)
	ip := &layers.IPv4{
		DstIP:    dst.IP,
		SrcIP:    src.IP,
		Version:  4,
		TTL:      64,
		Protocol: layers.IPProtocolUDP,
	}
	udp := &layers.UDP{
		SrcPort: layers.UDPPort(src.Port),
		DstPort: layers.UDPPort(dst.Port),
	}
	if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
		return nil, fmt.Errorf("failed calc checksum: %s", err)
	}
	if err := gopacket.SerializeLayers(buffer, gopacket.SerializeOptions{ComputeChecksums: true, FixLengths: true}, ip, udp, payload); err != nil {
		return nil, fmt.Errorf("failed serialize packet: %s", err)
	}
	return buffer.Bytes(), nil
}

type listenResult struct {
	ok    bool
	err   string
	iface string
}

func main() {

	if len(os.Args) != 2 {
		fmt.Println("Missing destination IP. Use the IP address that this node is reachable from for this")
		fmt.Println("peertester <destination IPv4>")
		return
	}

	dstIp := net.ParseIP(os.Args[1])
	if dstIp == nil {
		fmt.Println("Invalid IP address entered")
		return
	}

	lst := make(chan listenResult)
	go listen(lst)

	ifaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}

	type failed struct {
		iface  string
		reason string
	}
	var failedArray = make([]failed, 0)

	for _, iface := range ifaces {
		time.Sleep(500 * time.Millisecond)
		sendOnIface(iface, dstIp)
		select {
		case res := <-lst:
			if res.ok {
				if res.iface == iface.Name {
					fmt.Println(" SUCCESS")
				} else {
					fmt.Printf(" FAIL Received from %s instead of %s \n", res.iface, iface.Name)
					failedArray = append(failedArray, failed{iface: iface.Name, reason: "Received from " + res.iface + " instead of " + iface.Name})
				}
			} else {
				fmt.Printf(" FAIL %s\n", res.err)
				failedArray = append(failedArray, failed{iface: iface.Name, reason: res.err})
			}
		case <-time.After(2 * time.Second):
			fmt.Println(" FAIL")
			failedArray = append(failedArray, failed{iface: iface.Name, reason: "No response"})
		}
	}

	for _, f := range failedArray {
		fmt.Println(f.iface, f.reason)
	}

}

func listen(lst chan listenResult) {
	addr := net.UDPAddr{
		Port: 5000,
		IP:   net.ParseIP("0.0.0.0"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		panic(err)
	}
	for {
		var buf = make([]byte, 1000)
		numRead, remote, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			panic(err)
		}
		buf = buf[:numRead]
		if !remote.IP.Equal(net.ParseIP("172.21.48.53")) {
			lst <- listenResult{ok: false, err: "Packet from incorrect source IP: " + remote.String()}
			continue
		}
		lst <- listenResult{ok: true, iface: string(buf)}
	}

}

func sendOnIface(iface net.Interface, dstIP net.IP) {
	conn, err := open(iface.Name)
	if err != nil {
		fmt.Printf("[%s] Error: %s", iface.Name, err)
		return
	}
	dst := &net.UDPAddr{
		IP:   dstIP,
		Port: 5000,
	}
	b, err := buildUDPPacket(dst, &net.UDPAddr{IP: net.ParseIP("172.21.48.53"), Port: 5000}, []byte(iface.Name))
	if err != nil {
		fmt.Printf("[%s] Error: %s", iface.Name, err)
		return
	}
	_, err = conn.WriteTo(b, &net.IPAddr{IP: dst.IP})
	if err != nil {
		fmt.Printf("[%s] Error: %s", iface.Name, err)
		return
	}
	fmt.Printf("[%s] Testing with destination: %s ", iface.Name, dst)
}
