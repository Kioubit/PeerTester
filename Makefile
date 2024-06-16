# Makefile for PeerTester
BINARY=peertester
BUILDFLAGS=-trimpath

build:
	go build -o bin/${BINARY} .

release:
	CGO_ENABLED=0 go build ${BUILDFLAGS} -o bin/${BINARY} .

clean:
	if [ -d "bin/" ]; then find bin/ -type f -delete ;fi
	if [ -d "bin/" ]; then rm -d bin/ ;fi
