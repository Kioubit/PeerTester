package peerTester

import (
	"net"
)

type testResult int

const (
	OK        testResult = iota
	Timeout   testResult = iota
	InvalidIP testResult = iota
)

var SourceIPv4 = net.ParseIP("172.20.0.53")
var SourceIPv6 = net.ParseIP("fd42:d42:d42:54::1")
var OutputJSON bool

func DetectDstFromLoopBack(targetCIDR *net.IPNet) net.IP {
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
