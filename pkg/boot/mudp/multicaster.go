package mudp

import (
	"log"
	"net"
	"strings"
)

const (
	maxDatagramSize = 8192
)

type multicaster struct {
	addr  *net.UDPAddr
	close chan chan error
	conn  *net.UDPConn
}

func NewMulticaster(addr string) (mc *multicaster, err error) {
	udpAddr, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return
	}
	return &multicaster{addr: udpAddr, close: make(chan chan error)}, nil
}

func (mc *multicaster) Listen(ready chan bool, handler func(*net.UDPAddr, int, []byte)) {
	conn, err := net.ListenMulticastUDP("udp4", nil, mc.addr)
	if err != nil {
		log.Fatal(err)
		return
	}

	mc.conn = conn
	ready <- true

	mc.conn.SetReadBuffer(maxDatagramSize)

	for {
		buffer := make([]byte, maxDatagramSize)
		numBytes, src, err := mc.conn.ReadFromUDP(buffer)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Fatal("ReadFromUDP failed:", err)
		}
		handler(src, numBytes, buffer)
	}
}

func (mc *multicaster) Multicast(data []byte) (int, error) {
	conn, err := net.DialUDP("udp4", nil, mc.addr)
	if err != nil {
		return 0, err
	}
	return conn.Write(data)
}

func (mc *multicaster) Close() {
	if mc.conn != nil {
		mc.conn.Close()
	}
}
