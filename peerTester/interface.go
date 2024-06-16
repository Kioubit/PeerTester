package peerTester

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

const packetCount uint8 = 2

func open(intFace *net.Interface) (int, error) {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, 0)
	if err != nil {
		return -1, fmt.Errorf("failed open socket: %s", err)
	}

	if intFace != nil {
		intType, _ := os.ReadFile("/sys/class/net/" + intFace.Name + "/type")
		if intType != nil {
			if strings.TrimSpace(string(intType)) != "65534" {
				return -1, fmt.Errorf("%s is not a layer 3 interface", intFace.Name)
			}
		}

		err = syscall.Bind(fd, &syscall.SockaddrLinklayer{
			Protocol: 0,
			Ifindex:  intFace.Index,
			Hatype:   0,
			Pkttype:  0,
			Halen:    0,
			Addr:     [8]byte{},
		})
		if err != nil {
			return -1, fmt.Errorf("could not bind socket to device: %s", err)
		}
	}
	return fd, nil
}

func sendOnInterface(intFace net.Interface, srcIP, srcIP6, dstIP, dstIP6 net.IP, counter int) ([]timeInfo, error) {
	dst := &net.UDPAddr{
		IP:   dstIP,
		Port: 5000,
	}
	src := &net.UDPAddr{
		IP:   srcIP,
		Port: 5000,
	}
	dst6 := &net.UDPAddr{
		IP:   dstIP6,
		Port: 5000,
	}
	src6 := &net.UDPAddr{
		IP:   srcIP6,
		Port: 5000,
	}

	conn, err := open(&intFace)
	if err != nil {
		return nil, err
	}
	defer func(fd int) {
		_ = syscall.Close(fd)
	}(conn)

	var measurements = make([]timeInfo, 0)
	var id = 0
	for i := 0; i < int(packetCount); i++ {
		var contents = make([]byte, 3)
		contents[0] = byte(uint16(counter) >> 8)
		contents[1] = byte(uint16(counter))

		// IPv4
		contents[2] = byte(id)
		toSendV4 := hmacSeal([16]byte(hmacKey), contents)

		b4, err := buildUDPPacket(dst, src, toSendV4)
		if err != nil {
			return nil, err
		}

		var t time.Time
		var errorFirst = false
		t, err = sendOnInterfaceBytes(conn, b4)
		if err != nil {
			errorFirst = true
		}
		measurements = append(measurements, timeInfo{
			id:   uint8(id),
			time: t,
		})
		// ---------------

		time.Sleep(15 * time.Millisecond)

		// IPv6
		id++
		contents[2] = byte(id)
		toSendV6 := hmacSeal([16]byte(hmacKey), contents)

		b6, err := buildUDPPacket6(dst6, src6, toSendV6)
		if err != nil {
			return nil, err
		}

		t, err = sendOnInterfaceBytes(conn, b6)
		if err != nil {
			if errorFirst {
				return nil, err
			}
		}
		measurements = append(measurements, timeInfo{
			id:   uint8(id),
			time: t,
		})
		// ---------------

		if i != int(packetCount) {
			time.Sleep(15 * time.Millisecond)
		}
		id++
	}
	return measurements, nil
}

type timeInfo struct {
	id   uint8
	time time.Time
}

func sendOnInterfaceBytes(conn int, packetBytes []byte) (time.Time, error) {
	t := time.Now()
	_, err := syscall.Write(conn, packetBytes)
	if err != nil {
		return t, err
	}

	return t, nil
}
