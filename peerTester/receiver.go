package peerTester

import (
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func listenIntoChannel(lst chan *ListenResult, stopChannel chan bool, wg *sync.WaitGroup, readyToListen *sync.WaitGroup) {
	wg.Add(1)
	var stopping atomic.Bool
	addr := net.UDPAddr{
		Port: 5000,
		IP:   net.ParseIP(":"),
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		fmt.Printf(err.Error())
		os.Exit(1)
	}
	if !OutputJSON {
		fmt.Println("Listening on udp port 5000")
	}

	go func() {
		<-stopChannel
		stopping.Store(true)
		_ = conn.SetDeadline(time.Now())
		return
	}()

	var monotonic uint16 = 0

	readyToListen.Done()
	for {
		var buf = make([]byte, 1000)
		numRead, remote, err := conn.ReadFromUDP(buf[:])
		receiveTime := time.Now()
		if err != nil {
			if stopping.Load() {
				break
			} else {
				fmt.Printf("UDP receive error %s \n", err)
				os.Exit(1)
			}
		}
		buf = buf[:numRead]
		opened := hmacOpen([16]byte(hmacKey), buf)
		if opened == nil {
			// Received message with invalid hmac
			continue
		}

		if len(opened) != 3 {
			continue
		}
		counter := uint16(opened[1]) | uint16(opened[0])<<8
		id := opened[2]

		if counter > monotonic {
			monotonic = counter
		} else if counter < monotonic {
			continue
		}

		isV4 := remote.IP.To4() != nil

		lst <- &ListenResult{
			remoteIP: remote.IP,
			receiveTime: timeInfo{
				id:   id,
				time: receiveTime,
			},
			isV4: isV4,
		}
	}
	_ = conn.Close()
	close(lst)
	wg.Done()
}
