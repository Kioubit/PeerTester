package main

import (
	"fmt"
	"net"
	"os"
	"strings"
	"syscall"
	"time"
)

func open(ifName string) (int, error) {
	fd, err := syscall.Socket(syscall.AF_PACKET, syscall.SOCK_RAW, 0)
	if err != nil {
		return -1, fmt.Errorf("failed open socket: %s", err)
	}

	if ifName != "" {
		ni, err := net.InterfaceByName(ifName)
		if err != nil {
			return -1, fmt.Errorf("failed to find interface: %s: %s", ifName, err)
		}

		intType, _ := os.ReadFile("/sys/class/net/" + ifName + "/type")
		if intType != nil {
			if strings.TrimSpace(string(intType)) != "65534" {
				return -1, fmt.Errorf("%s is not a layer 3 interface", ifName)
			}
		}

		err = syscall.Bind(fd, &syscall.SockaddrLinklayer{
			Protocol: 0,
			Ifindex:  ni.Index,
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

func sendOnInterface(intFace net.Interface, srcIP, srcIP6, dstIP, dstIP6 net.IP) error {
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

	hmacKeyLock.Lock()
	toSend := hmacSeal([16]byte(hmacKey), []byte(intFace.Name))
	hmacKeyLock.Unlock()

	b4, err := buildUDPPacket(dst, src, toSend)
	if err != nil {
		return err
	}

	b6, err := buildUDPPacket6(dst6, src6, toSend)
	if err != nil {
		return err
	}

	_ = sendOnInterfaceBytes(intFace, b4, 2)
	_ = sendOnInterfaceBytes(intFace, b6, 2)
	return nil
}

func sendOnInterfaceBytes(intFace net.Interface, packetBytes []byte, repeat int) error {
	conn, err := open(intFace.Name)
	if err != nil {
		return err
	}
	defer func(fd int) {
		_ = syscall.Close(fd)
	}(conn)

	for i := 0; i < repeat; i++ {
		_, err = syscall.Write(conn, packetBytes)
		if err != nil {
			return err
		}
		if repeat != 1 {
			time.Sleep(20 * time.Millisecond)
		}
	}
	return nil
}
