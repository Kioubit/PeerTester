package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type listenResult struct {
	Status    testResult
	ErrorText string
	intFace   string
	remoteIP  net.IP
}

type testResult int

const (
	ok        testResult = iota
	timeout   testResult = iota
	invalidIP testResult = iota
)

var sourceIPv4 = net.ParseIP("172.20.0.53")
var sourceIPv6 = net.ParseIP("fd42:d42:d42:54::1")

func main() {
	destIPv4Str := flag.String("dst4", "", "destination IPv4 address (the address this host can be reached from) or CIDR to find address from 'lo'")
	destIPv6Str := flag.String("dst6", "", "destination IPv6 address (the address this host can be reached from) or CIDR to find address from 'lo'")
	targetInterface := flag.String("interface", "", "optional target interface")
	jsonOutput := flag.Bool("json", false, "output as JSON")
	flag.Parse()

	var dstIp, dstIp6 net.IP

	if strings.Contains(*destIPv4Str, "/") {
		_, cidr, err := net.ParseCIDR(*destIPv4Str)
		if err != nil {
			fmt.Printf("Error parsing IPv4 CIDR: %s\n", err)
			os.Exit(1)
		}
		dstIp = detectDstFromLoopBack(cidr)
		if dstIp == nil {
			fmt.Println("Could not find v4 address")
			os.Exit(1)
		}
		if !*jsonOutput {
			fmt.Println("Using destination IP:", dstIp.String())
		}
	} else {
		dstIp = net.ParseIP(*destIPv4Str)
		if dstIp == nil {
			if *destIPv4Str == "" {
				fmt.Println("No destination IPv4 address entered")
			} else {
				fmt.Println("Invalid IPv4 address entered")
			}
			os.Exit(1)
		}
	}

	if strings.Contains(*destIPv6Str, "/") {
		_, cidr, err := net.ParseCIDR(*destIPv6Str)
		if err != nil {
			fmt.Printf("Error parsing IPv6 CIDR: %s\n", err)
			os.Exit(1)
		}
		dstIp6 = detectDstFromLoopBack(cidr)
		if dstIp6 == nil {
			fmt.Println("Could not find v6 address")
			os.Exit(1)
		}
		if !*jsonOutput {
			fmt.Println("Using destination IP:", dstIp6.String())
		}
	} else {
		dstIp6 = net.ParseIP(*destIPv6Str)
		if dstIp6 == nil {
			if *destIPv4Str == "" {
				fmt.Println("No destination IPv6 address entered")
			} else {
				fmt.Println("Invalid IPv6 address entered")
			}
			os.Exit(1)
		}
	}

	type intFaceResult struct {
		V4 *listenResult
		V6 *listenResult
	}

	newHmacKey()
	var listenResultChannel = make(chan *listenResult, 4)
	var stopChannel = make(chan bool)
	var stopWG sync.WaitGroup

	go listenIntoChannel(listenResultChannel, stopChannel, &stopWG)
	if !*jsonOutput {
		fmt.Println("Listening on udp port 5000")
	}

	var intFaces = make([]net.Interface, 0)
	if *targetInterface != "" {
		intFace, err := net.InterfaceByName(*targetInterface)
		if err != nil {
			fmt.Printf("Error finding interface %s: %s\n", *targetInterface, err)
			os.Exit(1)
		}
		intFaces = append(intFaces, *intFace)
	} else {
		var err error
		intFaces, err = net.Interfaces()
		if err != nil {
			fmt.Printf("Error getting interfaces: %s\n", err)
			os.Exit(1)
		}
	}

	var finalMap = make(map[string]*intFaceResult)
	for _, intFace := range intFaces {
		time.Sleep(10 * time.Millisecond)
		newHmacKey()
		fr := &intFaceResult{
			V4: &listenResult{Status: timeout, ErrorText: "timeout"},
			V6: &listenResult{Status: timeout, ErrorText: "timeout"},
		}
		finalMap[intFace.Name] = fr

		err := sendOnInterface(intFace, sourceIPv4, sourceIPv6, dstIp, dstIp6)
		if err != nil {
			fmt.Printf(" -- Error sending on interface %s: %s\n", intFace.Name, err)
		}
		if !*jsonOutput {
			fmt.Printf("[%s] ", intFace.Name)
		}

		timeoutChan := time.After(2 * time.Second)
		for i := 0; i < 4; i++ {
			timedOut := false
			select {
			case result := <-listenResultChannel:
				if result.intFace != intFace.Name {
					continue
				}
				if result.remoteIP.To4() == nil {
					// IPv6
					if result.remoteIP.Equal(sourceIPv6) {
						if !*jsonOutput {
							fmt.Print(" SUCCESS (v6) ")
						}
						result.Status = ok
					} else {
						if !*jsonOutput {
							fmt.Print(" FAIL (v6) ")
						}
						result.ErrorText = "Invalid source IP: " + result.remoteIP.String()
						result.Status = invalidIP
					}
					fr.V6 = result
				} else {
					if result.remoteIP.Equal(sourceIPv4) {
						if !*jsonOutput {
							fmt.Print(" SUCCESS (v4) ")
						}
						result.Status = ok
					} else {
						if !*jsonOutput {
							fmt.Print(" FAIL (v4) ")
						}
						result.ErrorText = "Invalid source IP: " + result.remoteIP.String()
						result.Status = invalidIP
					}
					fr.V4 = result
				}
			case <-timeoutChan:
				if !*jsonOutput {
					fmt.Print(" TIMEOUT ")
				}
				timedOut = true
			}
			if timedOut {
				break
			}
		}
		if !*jsonOutput {
			fmt.Println()
		}
	}

	close(stopChannel)
	wgWaitTimout(&stopWG, 2*time.Second)

	if *jsonOutput {
		js, err := json.Marshal(finalMap)
		if err != nil {
			fmt.Printf("Error serializing map to JSON: %s\n", err)
			os.Exit(1)
		}
		fmt.Print(string(js))
		return
	}

	fmt.Println("-- Failed interface summary --")
	for intFaceName, result := range finalMap {
		var errors = make([]string, 0)
		if result.V4.Status != ok {
			errors = append(errors, fmt.Sprintf("Error (v4): %s", result.V4.ErrorText))
		}
		if result.V6.Status != ok {
			errors = append(errors, fmt.Sprintf("Error (v6): %s", result.V6.ErrorText))
		}
		if len(errors) != 0 {
			fmt.Printf("[%s] %s", intFaceName, strings.Join(errors, " "))
			fmt.Println()
		}
	}
}

func listenIntoChannel(lst chan *listenResult, stopChannel chan bool, wg *sync.WaitGroup) {
	wg.Add(1)
	var stopping atomic.Bool
	addr := net.UDPAddr{
		Port: 5000,
		IP:   net.ParseIP(":"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		panic(err)
	}

	go func() {
		<-stopChannel
		stopping.Store(true)
		_ = conn.SetDeadline(time.Now())
		return
	}()

	for {
		var buf = make([]byte, 1000)
		numRead, remote, err := conn.ReadFromUDP(buf[:])
		if err != nil {
			if stopping.Load() {
				break
			} else {
				fmt.Printf("UDP receive error %s \n", err)
				os.Exit(1)
			}
		}
		buf = buf[:numRead]
		hmacKeyLock.Lock()
		opened := hmacOpen([16]byte(hmacKey), buf)
		hmacKeyLock.Unlock()
		if opened == nil {
			// Received message with invalid hmac
			continue
		}
		lst <- &listenResult{intFace: string(opened), remoteIP: remote.IP}
	}
	_ = conn.Close()
	close(lst)
	wg.Done()
}

func detectDstFromLoopBack(targetCIDR *net.IPNet) net.IP {
	loopBack, err := net.InterfaceByName("lo")
	if err != nil {
		return nil
	}

	addresses, err := loopBack.Addrs()
	if err != nil {
		return nil
	}
	for _, addr := range addresses {
		testIP, _, err := net.ParseCIDR(addr.String())
		if err != nil || testIP == nil {
			continue
		}
		if targetCIDR.Contains(testIP) {
			return testIP
		}
	}
	return nil
}
