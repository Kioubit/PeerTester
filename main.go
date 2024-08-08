package main

import (
	"PeerTester/peerTester"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

func main() {
	destIPv4Str := flag.String("dst4", "", "destination IPv4 address "+
		"(the address this host can be reached from) or CIDR to find address from 'lo'")
	destIPv6Str := flag.String("dst6", "", "destination IPv6 address "+
		"(the address this host can be reached from) or CIDR to find address from 'lo'")
	targetInterface := flag.String("interface", "", "optional comma-separated target interface(s). "+
		"Use '-' to read from stdin. If not specified, packets are sent on all interfaces")
	jsonOutput := flag.Bool("json", false, "output as JSON")
	daemon := flag.Bool("daemon", false, "run as a daemon and accept interface lists via unix socket")
	flag.Parse()

	peerTester.OutputJSON = *jsonOutput

	var dstIp, dstIp6 net.IP

	if strings.Contains(*destIPv4Str, "/") {
		_, cidr, err := net.ParseCIDR(*destIPv4Str)
		if err != nil {
			fmt.Printf("Error parsing IPv4 CIDR: %s\n", err)
			os.Exit(1)
		}
		dstIp = peerTester.DetectDstFromLoopBack(cidr)
		if dstIp == nil {
			fmt.Println("Could not find v4 address")
			os.Exit(1)
		}
		if !peerTester.OutputJSON {
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
		dstIp6 = peerTester.DetectDstFromLoopBack(cidr)
		if dstIp6 == nil {
			fmt.Println("Could not find v6 address")
			os.Exit(1)
		}
		if !peerTester.OutputJSON {
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

	if *daemon {
		runAsDaemon(dstIp, dstIp6)
	} else {
		runAsCli(dstIp, dstIp6, *targetInterface)
	}
}

func runAsCli(dstIp, dstIp6 net.IP, targetInterface string) {
	var intFaces = make([]net.Interface, 0)
	if targetInterface != "" {
		if targetInterface == "-" {
			_, err := fmt.Scanln(&targetInterface)
			if err != nil {
				fmt.Printf("Error reading from stdin: %s\n", err)
				os.Exit(1)
			}
		}

		intFaceListInput := strings.Split(targetInterface, ",")
		for _, intFaceInput := range intFaceListInput {
			intFace, err := net.InterfaceByName(intFaceInput)
			if err != nil {
				fmt.Printf("Error finding interface %s: %s\n", intFaceInput, err)
				os.Exit(1)
			}
			intFaces = append(intFaces, *intFace)
		}
	} else {
		var err error
		intFaces, err = net.Interfaces()
		if err != nil {
			fmt.Printf("Error getting interfaces: %s\n", err)
			os.Exit(1)
		}
	}
	resultMap := peerTester.PerformTests(intFaces, dstIp, dstIp6)

	if peerTester.OutputJSON {
		js, err := json.Marshal(resultMap)
		if err != nil {
			fmt.Printf("Error serializing map to JSON: %s\n", err)
			os.Exit(1)
		}
		fmt.Print(string(js))
		return
	}

	// Human-readable output
	if len(resultMap) > 0 {
		fmt.Println("-- Failed interface summary --")
		for intFaceName, result := range resultMap {
			var errors = make([]string, 0)
			if result.V4.Status != peerTester.OK {
				errors = append(errors, fmt.Sprintf("Error (v4): %s", result.V4.ErrorText))
			}
			if result.V6.Status != peerTester.OK {
				errors = append(errors, fmt.Sprintf("Error (v6): %s", result.V6.ErrorText))
			}
			if len(errors) != 0 {
				fmt.Printf("[%-10s] %s", intFaceName, strings.Join(errors, " "))
				fmt.Println()
			}
		}
	}
}

func runAsDaemon(dstIp, dstIp6 net.IP) {
	socket, err := net.Listen("unix", "peer-tester.sock")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		_ = os.Remove("peer-tester.sock")
		os.Exit(0)
	}()

	for {
		conn, err := socket.Accept()
		if err != nil {
			fmt.Printf("Error unix socket accepting connection: %s\n", err)
			os.Exit(1)
		}
		go func(conn net.Conn) {
			defer func(conn net.Conn) {
				_ = conn.Close()
			}(conn)

			err = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			if err != nil {
				fmt.Printf("Error setting unix socket read deadline: %s\n", err)
				return
			}
			buf := make([]byte, 10000)
			numRead, err := conn.Read(buf)
			if err != nil {
				fmt.Printf("Error reading from unix socket: %s\n", err)
				return
			}
			list := string(buf[:numRead])

			intFaces := make([]net.Interface, 0)
			for _, intFaceInput := range strings.Split(list, ",") {
				intFace, err := net.InterfaceByName(intFaceInput)
				if err != nil {
					fmt.Printf("Error finding interface %s: %s\n", intFaceInput, err)
					_, _ = conn.Write([]byte("error"))
				}
				intFaces = append(intFaces, *intFace)
			}

			resultMap := peerTester.PerformTests(intFaces, dstIp, dstIp6)
			js, err := json.Marshal(resultMap)
			if err != nil {
				fmt.Printf("Error serializing map to JSON: %s\n", err)
				os.Exit(1)
			}
			_, _ = conn.Write(js)
		}(conn)
	}
}
