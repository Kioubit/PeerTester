package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

type listenResult struct {
	ok         bool
	err        string
	fatalError bool
	intFace    string
	remoteIP   net.IP
}

var (
	hmacKey     []byte
	hmacKeyLock sync.Mutex
)

func main() {
	if len(os.Args) != 3 {
		fmt.Println("Missing destination IP. Use the IP address that this host is reachable from for this")
		fmt.Println("Arguments: <destination IPv4> <destination IPv6>")
		return
	}

	dstIp := net.ParseIP(os.Args[1])
	if dstIp == nil {
		fmt.Println("Invalid IPv4 address entered")
		return
	}

	dstIp6 := net.ParseIP(os.Args[2])
	if dstIp6 == nil {
		fmt.Println("Invalid IPv6 address entered")
		return
	}

	type intFaceResult struct {
		v4 *listenResult
		v6 *listenResult
	}

	newHmacKey()
	var listenResultChannel = make(chan *listenResult)
	var listenResultChannel6 = make(chan *listenResult)
	go listenIntoChannel(listenResultChannel, listenResultChannel6)

	intFaces, err := net.Interfaces()
	if err != nil {
		panic(err)
	}

	var finalMap = make(map[string]*intFaceResult)
	for _, intFace := range intFaces {
		time.Sleep(10 * time.Millisecond)
		newHmacKey()
		finalMap[intFace.Name] = &intFaceResult{
			v4: &listenResult{err: "timeout"},
			v6: &listenResult{err: "timeout"},
		}
		sendOnInterface(intFace, dstIp, dstIp6)
		for i := 0; i < 2; i++ {
			timedOut := false
			select {
			case re4 := <-listenResultChannel:
				evaluateListenResult(re4, intFace.Name, false)
				if re4.ok {
					fmt.Print(" SUCCESS (v4) ")
				} else {
					fmt.Print(" FAIL (v4) ")
				}
				finalMap[intFace.Name].v4 = re4
			case re6 := <-listenResultChannel6:
				evaluateListenResult(re6, intFace.Name, true)
				if re6.ok {
					fmt.Print(" SUCCESS (v6) ")
				} else {
					fmt.Print(" FAIL (6) ")
				}
				finalMap[intFace.Name].v6 = re6
			case <-time.After(2 * time.Second):
				fmt.Print(" TIMEOUT ")
				timedOut = true
			}
			if timedOut {
				break
			}
		}
		fmt.Println()
	}
	fmt.Println("-- Failed interface summary --")

	for intFaceName, result := range finalMap {
		if (!result.v6.ok && !result.v4.ok) || (result.v4.fatalError || result.v6.fatalError) {
			fmt.Printf("[%s] Error (v4): %s Error (v6): %s\n", intFaceName, result.v4.err, result.v6.err)
		}
	}
}

func evaluateListenResult(result *listenResult, intFace string, isV6 bool) {
	expectedIP := "172.20.0.53"
	if isV6 {
		expectedIP = "fd42:d42:d42:54::1"
	}
	if !result.remoteIP.Equal(net.ParseIP(expectedIP)) {
		result.err = "Packet from incorrect source IP: " + result.remoteIP.String()
		result.fatalError = true
		return
	}

	if result.intFace != intFace {
		result.err = "Received from interface: " + result.intFace + " instead of " + intFace
		result.fatalError = true
		return
	}
	result.ok = true
}

func listenIntoChannel(lst chan *listenResult, lst6 chan *listenResult) {
	addr := net.UDPAddr{
		Port: 5000,
		IP:   net.ParseIP(":"),
	}
	fmt.Println("Listening on udp port 5000")
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
		hmacKeyLock.Lock()
		opened := hmacOpen([16]byte(hmacKey), buf)
		hmacKeyLock.Unlock()
		if opened == nil {
			fmt.Print(" received message with invalid hmac ")
			continue
		}
		if !strings.Contains(remote.IP.String(), ":") {
			lst <- &listenResult{intFace: string(opened), remoteIP: remote.IP}
		} else {
			lst6 <- &listenResult{intFace: string(opened), remoteIP: remote.IP}
		}
	}
}
