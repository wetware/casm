package multicast

import ma "github.com/multiformats/go-multiaddr"

const (
	// TODO:  register protocol at https://github.com/multiformats/multicodec
	P_MCAST         = 0x07
	maxDatagramSize = 8192
)

func init() {
	if err := ma.AddProtocol(ma.Protocol{
		Name:       "multicast",
		Code:       P_MCAST,
		VCode:      ma.CodeToVarint(P_MCAST),
		Size:       ma.LengthPrefixedVarSize,
		Path:       true,
		Transcoder: ma.NewTranscoderFromFunctions(mcastStoB, mcastBtoS, nil),
	}); err != nil {
		panic(err)
	}
}

func mcastStoB(s string) ([]byte, error) {
	m, err := ma.NewMultiaddr(s)
	if err != nil {
		return nil, err
	}

	return m.Bytes(), nil
}

func mcastBtoS(b []byte) (string, error) {
	m, err := ma.NewMultiaddrBytes(b)
	if err != nil {
		return "", err
	}
	return m.String(), nil
}
