package peerTester

import (
	"encoding/binary"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func listenIntoChannel(lst chan *ListenResult, stopChannel chan bool, wg *sync.WaitGroup, readyToListen *sync.WaitGroup) {
	var stopping atomic.Bool
	addr := net.UDPAddr{
		Port: 5000,
	}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		fmt.Printf(err.Error())
		os.Exit(1)
	}
	if !OutputJSON {
		fmt.Println("Listening on udp port 5000")
	}

	rawConn, err := conn.SyscallConn()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	err = rawConn.Control(func(fd uintptr) {
		if err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, syscall.IPV6_RECVHOPLIMIT, 1); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
		if err := syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, syscall.IP_RECVTTL, 1); err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	})
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
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
		var buf = make([]byte, 1500)
		var oobBuf = make([]byte, 1500)
		numRead, numReadOOB, _, remote, err := conn.ReadMsgUDP(buf, oobBuf)
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

		ttlValue := parseOOBTTL(oobBuf[:numReadOOB])

		isV4 := remote.IP.To4() != nil

		lst <- &ListenResult{
			remoteIP: remote.IP,
			receiveTime: timeInfo{
				id:   id,
				time: receiveTime,
			},
			isV4:     isV4,
			ttlValue: ttlValue,
		}
	}
	_ = conn.Close()
	close(lst)
	wg.Done()
}

func parseOOBTTL(oobData []byte) (ttl int32) {
	cMSGs, err := syscall.ParseSocketControlMessage(oobData)
	if err != nil {
		return -1
	}
	for _, msg := range cMSGs {
		if msg.Header.Type == syscall.IP_TTL {
			return int32(binary.NativeEndian.Uint32(msg.Data))
		}
		if msg.Header.Type == syscall.IPV6_HOPLIMIT {
			return int32(binary.NativeEndian.Uint32(msg.Data))
		}
	}
	return -1
}
