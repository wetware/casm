package boot

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/libp2p/go-libp2p-core/record"
)

var (
	// ErrSkip should be returned from Scanner implementations to signal
	// that the ScanStrategy should continue to the next address without
	// returning.
	ErrSkip = errors.New("skip")
)

type Dialer interface {
	DialContext(ctx context.Context, network, addr string) (net.Conn, error)
}

type ScanStrategy interface {
	Scan(context.Context, Dialer, record.Record) (*record.Envelope, error)
}

type Scanner interface {
	Scan(net.Conn, record.Record) (*record.Envelope, error)
}

type ScanSubnet struct {
	Net  string
	Port int
	Subnet
	Scanner Scanner
}

func (iter *ScanSubnet) Network() string {
	if iter.Net == "" {
		return "tcp"
	}

	return iter.Net
}

func (iter *ScanSubnet) String() string {
	var ip = make(net.IP, 4)
	iter.Subnet.Scan(ip)
	return fmt.Sprintf("%s:%d",
		ip.String(),
		iter.Port)
}

func (iter *ScanSubnet) Scan(ctx context.Context, d Dialer, r record.Record) (msg *record.Envelope, err error) {
	for iter.Subnet.Reset(); iter.More(); iter.Next() {
		if iter.Err != nil {
			break
		}

		if iter.Skip() {
			continue
		}

		if msg, err = iter.scan(ctx, d, r); err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}

			break
		}
	}

	if err != nil {
		err = iter.Err
	}

	return
}

func (iter *ScanSubnet) scan(ctx context.Context, d Dialer, r record.Record) (*record.Envelope, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Millisecond*10)
	defer cancel()

	conn, err := iter.dial(ctx, d)
	if err != nil {
		return nil, err
	}

	return iter.Scanner.Scan(conn, r)
}

func (iter *ScanSubnet) dial(ctx context.Context, d Dialer) (net.Conn, error) {
	var ip = make(net.IP, 4)
	iter.Subnet.Scan(ip[:])
	return d.DialContext(ctx,
		iter.Network(),
		fmt.Sprintf("%v:%d", ip, iter.Port))
}
