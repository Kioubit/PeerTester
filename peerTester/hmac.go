package peerTester

import (
	"crypto/hmac"
	cryptorand "crypto/rand"
	"crypto/sha256"
)

var (
	hmacKey []byte
)

func hmacSeal(key [16]byte, message []byte) []byte {
	mac := hmac.New(sha256.New, key[:])
	mac.Write(message)
	return mac.Sum(message)
}

func hmacOpen(key [16]byte, macMessage []byte) []byte {
	if len(macMessage) <= sha256.Size {
		return nil
	}
	message := macMessage[:len(macMessage)-sha256.Size]
	macPart := macMessage[len(macMessage)-sha256.Size:]
	mac := hmac.New(sha256.New, key[:])
	mac.Write(message)
	expectedMac := mac.Sum(nil)
	if !hmac.Equal(expectedMac, macPart) {
		return nil
	}
	return message
}

func newHmacKey() {
	hmacKey = make([]byte, 16)
	_, err := cryptorand.Read(hmacKey)
	if err != nil {
		panic(err)
	}
}
