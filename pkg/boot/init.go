package boot

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"unsafe"

	"github.com/libp2p/go-libp2p-core/discovery"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/multiformats/go-multiaddr"
	"github.com/multiformats/go-varint"
	"github.com/wetware/casm/pkg/boot/survey"
)

/* var (
	P_CRAWL multiaddr.Protocol{name: "crawl", code:}
) */

const (
	P_CRAWL_CODE   = 100
	P_SURVEY_CODE  = 101
	P_GRADUAL_CODE = 102
)

var (
	P_CRAWL   = multiaddr.Protocol{"crawl", P_CRAWL_CODE, varint.ToUvarint(P_CRAWL_CODE), -1, true, CrawlTranscoder{}}
	P_SURVEY  = multiaddr.Protocol{"survey", P_SURVEY_CODE, varint.ToUvarint(P_SURVEY_CODE), 0, false, nil}
	P_GRADUAL = multiaddr.Protocol{"gradual", P_GRADUAL_CODE, varint.ToUvarint(P_GRADUAL_CODE), 0, false, nil}
)

func init() {
	multiaddr.AddProtocol(P_CRAWL)
	multiaddr.AddProtocol(P_SURVEY)
	multiaddr.AddProtocol(P_GRADUAL)
}

func getNetwork(maddr multiaddr.Multiaddr) (isIp4 bool, ip net.IP, err error) {
	ip4, err := maddr.ValueForProtocol(multiaddr.P_IP4)
	if err == nil {
		return true, net.ParseIP(ip4), nil
	}
	ip6, err := maddr.ValueForProtocol(multiaddr.P_IP6)
	if err == nil {
		return false, net.ParseIP(ip6), nil
	}
	return isIp4, ip, errors.New("no network")
}

func getTransport(maddr multiaddr.Multiaddr) (isUdp bool, num int, err error) {
	udp, err := maddr.ValueForProtocol(multiaddr.P_UDP)
	if err == nil {
		num, err = strconv.Atoi(udp)
		return
	}

	tcp, err := maddr.ValueForProtocol(multiaddr.P_TCP)
	if err == nil {
		num, err = strconv.Atoi(tcp)
		return
	}

	return isUdp, num, errors.New("no transport")
}

func parse(h host.Host, maddr multiaddr.Multiaddr) (discovery.Discoverer, error) {
	var addr net.Addr

	isIp4, n, err := getNetwork(maddr)
	if err != nil {
		return nil, err
	}

	isUdp, t, err := getTransport(maddr)
	if err != nil {
		return nil, err
	}

	if isUdp && isIp4 {
		addr, err = net.ResolveUDPAddr("udp4", fmt.Sprintf("%v:%v", n, t))
		if err != nil {
			return nil, err
		}
	} else if isUdp && !isIp4 {
		addr, err = net.ResolveUDPAddr("udp6", fmt.Sprintf("%v:%v", n, t))
		if err != nil {
			return nil, err
		}
	} else if !isUdp && isIp4 {
		addr, err = net.ResolveTCPAddr("tcp4", fmt.Sprintf("%v:%v", n, t))
		if err != nil {
			return nil, err
		}
	} else {
		addr, err = net.ResolveTCPAddr("tcp6", fmt.Sprintf("%v:%v", n, t))
		if err != nil {
			return nil, err
		}
	}

	c, err := getCrawl(maddr)
	if err != nil {
		return nil, err
	} else {
		// TODO: initialize crawler
	}

	err = getSurvey(maddr)
	if err != nil {
		return nil, err
	} else {
		surv, err := survey.New(h, addr)
		if err != nil {
			return nil, err
		}

		if err = getGradual(maddr); err == nil {
			return survey.GradualSurveyor{Surveyor: surv}, nil
		} else {
			return surv, nil
		}

	}
}

func getSurvey(maddr multiaddr.Multiaddr) error {
	_, err := maddr.ValueForProtocol(P_SURVEY_CODE)
	return err
}

func getGradual(maddr multiaddr.Multiaddr) error {
	_, err := maddr.ValueForProtocol(P_GRADUAL_CODE)
	return err
}

func getCrawl(maddr multiaddr.Multiaddr) (int, error) {
	c, err := maddr.ValueForProtocol(P_CRAWL_CODE)
	if err != nil {
		return 0, err
	}
	valBytes, err := P_CRAWL.Transcoder.StringToBytes(c)
	if err != nil {
		return 0, err
	}
	return ByteArrayToInt(valBytes), nil
}

type CrawlTranscoder struct{}

func (ct CrawlTranscoder) StringToBytes(cidrBlock string) ([]byte, error) {
	num, err := strconv.Atoi(cidrBlock)
	if err != nil {
		return nil, err
	}
	if num > 128 || num < 0 {
		return nil, errors.New("invalid CIDR block")
	}
	return IntToByteArray(num), nil
}

func (ct CrawlTranscoder) BytesToString(b []byte) (string, error) {
	num := ByteArrayToInt(b)
	return strconv.Itoa(num), nil
}

func (ct CrawlTranscoder) ValidateBytes(b []byte) error {
	if num := ByteArrayToInt(b); num < 128 || num < 0 { // 128 is maximum CIDR block for IPv6
		return nil
	} else {
		return errors.New("invalid CIDR block")
	}
}

func IntToByteArray(num int) []byte {
	size := int(unsafe.Sizeof(num))
	arr := make([]byte, size)
	for i := 0; i < size; i++ {
		byt := *(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(&num)) + uintptr(i)))
		arr[i] = byt
	}
	return arr
}

func ByteArrayToInt(arr []byte) int {
	val := int(0)
	size := len(arr)
	for i := 0; i < size; i++ {
		*(*uint8)(unsafe.Pointer(uintptr(unsafe.Pointer(&val)) + uintptr(i))) = arr[i]
	}
	return val
}
