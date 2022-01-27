package mudp

import (
	"log"
	"net"
	"strings"
)

const (
	maxDatagramSize = 8192
)

type DialFunc func(laddr, raddr *net.UDPAddr) (*net.UDPConn, error)
type ListenFunc func(laddr *net.UDPAddr) (*net.UDPConn, error)

type multicaster struct {
	addr   *net.UDPAddr
	dial   DialFunc
	listen ListenFunc
	conn   *net.UDPConn
}

func NewMulticaster(addr *net.UDPAddr, dial DialFunc, listen ListenFunc) (mc *multicaster, err error) {
	return &multicaster{addr: addr, dial: dial, listen: listen}, nil
}

func (mc *multicaster) Listen(ready chan bool, handler func(int, net.Addr, []byte)) {
	conn, err := mc.listen(mc.addr)
	if err != nil {
		log.Fatal(err)
		return
	}

	mc.conn = conn
	ready <- true

	for {
		buffer := make([]byte, maxDatagramSize)
		numBytes, src, err := mc.conn.ReadFrom(buffer)
		if err != nil {
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			log.Fatal("ReadFromUDP failed:", err)
		}
		handler(numBytes, src, buffer)
	}
}

func (mc *multicaster) Multicast(data []byte) (int, error) {
	conn, err := mc.dial(nil, mc.addr)
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
