package peerTester

import (
	"fmt"
	"math"
	"net"
	"sync"
	"syscall"
	"time"
)

type ListenResult struct {
	Status      testResult
	ErrorText   string
	Latency     int
	PacketsLost int
	receiveTime timeInfo
	isV4        bool
	remoteIP    net.IP
}

type IntFaceResult struct {
	V4 *ListenResult
	V6 *ListenResult
}

func PerformTests(intFaces []net.Interface, dstIp net.IP, dstIp6 net.IP) (resultMap map[string]*IntFaceResult) {
	setHighPriority()
	resultMap = make(map[string]*IntFaceResult)
	newHmacKey()

	var listenResultChannel = make(chan *ListenResult, 4)
	var stopChannel = make(chan bool)
	var stopWG sync.WaitGroup

	var readyToListen sync.WaitGroup
	readyToListen.Add(1)
	go listenIntoChannel(listenResultChannel, stopChannel, &stopWG, &readyToListen)
	readyToListen.Wait()

	var interFaceCount = len(intFaces)
	if interFaceCount > math.MaxInt16 {
		if !OutputJSON {
			fmt.Println("Warning: Too many interfaces. Truncating list.")
		}
		intFaces = intFaces[:math.MaxInt16]
	}

	for counter, intFace := range intFaces {
		r := testInterface(intFace, listenResultChannel, dstIp, dstIp6, counter)
		if !OutputJSON {
			fmt.Printf("[%-10s] V4: %-7s (%-3dms - Lost %d pkts) V6: %-7s (%-3dms - Lost %d pkts)\n", intFace.Name, r.V4.ErrorText, r.V4.Latency, r.V4.PacketsLost, r.V6.ErrorText, r.V6.Latency, r.V6.PacketsLost)
		}
		resultMap[intFace.Name] = r

		if counter != interFaceCount {
			time.Sleep(10 * time.Millisecond)
		}
	}

	close(stopChannel) // Request listener stop

	wgWaitTimout(&stopWG, 2*time.Second)
	return
}

func testInterface(intFace net.Interface, listenResultChannel chan *ListenResult, dstIp net.IP, dstIp6 net.IP, counter int) *IntFaceResult {
	fr := &IntFaceResult{
		V4: &ListenResult{Status: Timeout, ErrorText: "timeout", Latency: -1},
		V6: &ListenResult{Status: Timeout, ErrorText: "timeout", Latency: -1},
	}

	var doneWg sync.WaitGroup
	var sendMeasurements []timeInfo
	var receiveResults = make([]*ListenResult, 0)
	var skipChannel = make(chan bool, 1)
	defer close(skipChannel)

	go func() {
		doneWg.Add(1)
		defer doneWg.Done()

		var err error
		sendMeasurements, err = sendOnInterface(intFace, SourceIPv4, SourceIPv6, dstIp, dstIp6, counter)
		if err != nil {
			if !OutputJSON {
				fmt.Printf(" -- Error sending on interface %s: %s\n", intFace.Name, err)
			}
			skipChannel <- true
		}
	}()

	timeoutChan := time.After(2 * time.Second)
	for i := 0; i < 4; i++ {
		timedOut := false
		select {
		case result := <-listenResultChannel:
			receiveResults = append(receiveResults, result)
		case <-timeoutChan:
			timedOut = true
		case <-skipChannel:
			timedOut = true
		}
		if timedOut {
			break
		}
	}

	doneWg.Wait()

	if sendMeasurements == nil {
		return fr
	}

	v4Latencies := make([]int, 0)
	v6Latencies := make([]int, 0)

	for _, result := range receiveResults {
		if result.isV4 {
			if result.remoteIP.Equal(SourceIPv4) {
				result.Status = OK
				result.ErrorText = "OK"
			} else {
				result.ErrorText = "Invalid source IP: " + result.remoteIP.String()
				result.Status = InvalidIP
			}
			fr.V4 = result
		} else {
			if result.remoteIP.Equal(SourceIPv6) {
				result.Status = OK
				result.ErrorText = "OK"
			} else {
				result.ErrorText = "Invalid source IP: " + result.remoteIP.String()
				result.Status = InvalidIP
			}
			fr.V6 = result
		}
		// Latency recording
		for _, sendMeasurement := range sendMeasurements {
			if result.receiveTime.id == sendMeasurement.id {
				// Found corresponding measurement
				if result.isV4 {
					v4Latencies = append(v4Latencies, int(result.receiveTime.time.Sub(sendMeasurement.time).Milliseconds()))
				} else {
					v6Latencies = append(v6Latencies, int(result.receiveTime.time.Sub(sendMeasurement.time).Milliseconds()))
				}
				break
			}
		}
	}

	if len(v4Latencies) != 0 {
		var sum = 0
		for i := 0; i < len(v4Latencies); i++ {
			sum += max(v4Latencies[i], 0)
		}
		fr.V4.Latency = sum / len(v4Latencies)
	}

	if len(v6Latencies) != 0 {
		var sum = 0
		for i := 0; i < len(v6Latencies); i++ {
			sum += max(v6Latencies[i], 0)
		}
		fr.V6.Latency = sum / len(v6Latencies)
	}

	fr.V6.PacketsLost = int(packetCount) - len(v6Latencies)
	fr.V4.PacketsLost = int(packetCount) - len(v4Latencies)

	return fr
}

func setHighPriority() {
	// Try setting higher process priority to reduce any inaccuracies during latency measurement
	const (
		pidSelf       = 0
		wantNiceLevel = -10
	)
	if cur, err := syscall.Getpriority(syscall.PRIO_PROCESS, pidSelf); err == nil && cur <= wantNiceLevel {
		return
	}

	if err := syscall.Setpriority(syscall.PRIO_PROCESS, pidSelf, wantNiceLevel); err != nil {
		return
	}
	return
}
