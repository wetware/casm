// Code generated by capnpc-go. DO NOT EDIT.

package boot

import (
	capnp "capnproto.org/go/capnp/v3"
	text "capnproto.org/go/capnp/v3/encoding/text"
	schemas "capnproto.org/go/capnp/v3/schemas"
	strconv "strconv"
)

type MulticastPacket struct{ capnp.Struct }
type MulticastPacket_Which uint16

const (
	MulticastPacket_Which_query    MulticastPacket_Which = 0
	MulticastPacket_Which_response MulticastPacket_Which = 1
)

func (w MulticastPacket_Which) String() string {
	const s = "queryresponse"
	switch w {
	case MulticastPacket_Which_query:
		return s[0:5]
	case MulticastPacket_Which_response:
		return s[5:13]

	}
	return "MulticastPacket_Which(" + strconv.FormatUint(uint64(w), 10) + ")"
}

// MulticastPacket_TypeID is the unique identifier for the type MulticastPacket.
const MulticastPacket_TypeID = 0xf859740361623722

func NewMulticastPacket(s *capnp.Segment) (MulticastPacket, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return MulticastPacket{st}, err
}

func NewRootMulticastPacket(s *capnp.Segment) (MulticastPacket, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return MulticastPacket{st}, err
}

func ReadRootMulticastPacket(msg *capnp.Message) (MulticastPacket, error) {
	root, err := msg.Root()
	return MulticastPacket{root.Struct()}, err
}

func (s MulticastPacket) String() string {
	str, _ := text.Marshal(0xf859740361623722, s.Struct)
	return str
}

func (s MulticastPacket) Which() MulticastPacket_Which {
	return MulticastPacket_Which(s.Struct.Uint16(0))
}
func (s MulticastPacket) Query() (string, error) {
	if s.Struct.Uint16(0) != 0 {
		panic("Which() != query")
	}
	p, err := s.Struct.Ptr(0)
	return p.Text(), err
}

func (s MulticastPacket) HasQuery() bool {
	if s.Struct.Uint16(0) != 0 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s MulticastPacket) QueryBytes() ([]byte, error) {
	p, err := s.Struct.Ptr(0)
	return p.TextBytes(), err
}

func (s MulticastPacket) SetQuery(v string) error {
	s.Struct.SetUint16(0, 0)
	return s.Struct.SetText(0, v)
}

func (s MulticastPacket) Response() (MulticastPacket_Response, error) {
	if s.Struct.Uint16(0) != 1 {
		panic("Which() != response")
	}
	p, err := s.Struct.Ptr(0)
	return MulticastPacket_Response{Struct: p.Struct()}, err
}

func (s MulticastPacket) HasResponse() bool {
	if s.Struct.Uint16(0) != 1 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s MulticastPacket) SetResponse(v MulticastPacket_Response) error {
	s.Struct.SetUint16(0, 1)
	return s.Struct.SetPtr(0, v.Struct.ToPtr())
}

// NewResponse sets the response field to a newly
// allocated MulticastPacket_Response struct, preferring placement in s's segment.
func (s MulticastPacket) NewResponse() (MulticastPacket_Response, error) {
	s.Struct.SetUint16(0, 1)
	ss, err := NewMulticastPacket_Response(s.Struct.Segment())
	if err != nil {
		return MulticastPacket_Response{}, err
	}
	err = s.Struct.SetPtr(0, ss.Struct.ToPtr())
	return ss, err
}

// MulticastPacket_List is a list of MulticastPacket.
type MulticastPacket_List struct{ capnp.List }

// NewMulticastPacket creates a new list of MulticastPacket.
func NewMulticastPacket_List(s *capnp.Segment, sz int32) (MulticastPacket_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1}, sz)
	return MulticastPacket_List{l}, err
}

func (s MulticastPacket_List) At(i int) MulticastPacket { return MulticastPacket{s.List.Struct(i)} }

func (s MulticastPacket_List) Set(i int, v MulticastPacket) error {
	return s.List.SetStruct(i, v.Struct)
}

func (s MulticastPacket_List) String() string {
	str, _ := text.MarshalList(0xf859740361623722, s.List)
	return str
}

// MulticastPacket_Future is a wrapper for a MulticastPacket promised by a client call.
type MulticastPacket_Future struct{ *capnp.Future }

func (p MulticastPacket_Future) Struct() (MulticastPacket, error) {
	s, err := p.Future.Struct()
	return MulticastPacket{s}, err
}

func (p MulticastPacket_Future) Response() MulticastPacket_Response_Future {
	return MulticastPacket_Response_Future{Future: p.Future.Field(0, nil)}
}

type MulticastPacket_Response struct{ capnp.Struct }

// MulticastPacket_Response_TypeID is the unique identifier for the type MulticastPacket_Response.
const MulticastPacket_Response_TypeID = 0xb998800b6731ad22

func NewMulticastPacket_Response(s *capnp.Segment) (MulticastPacket_Response, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 0, PointerCount: 2})
	return MulticastPacket_Response{st}, err
}

func NewRootMulticastPacket_Response(s *capnp.Segment) (MulticastPacket_Response, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 0, PointerCount: 2})
	return MulticastPacket_Response{st}, err
}

func ReadRootMulticastPacket_Response(msg *capnp.Message) (MulticastPacket_Response, error) {
	root, err := msg.Root()
	return MulticastPacket_Response{root.Struct()}, err
}

func (s MulticastPacket_Response) String() string {
	str, _ := text.Marshal(0xb998800b6731ad22, s.Struct)
	return str
}

func (s MulticastPacket_Response) Ns() (string, error) {
	p, err := s.Struct.Ptr(0)
	return p.Text(), err
}

func (s MulticastPacket_Response) HasNs() bool {
	return s.Struct.HasPtr(0)
}

func (s MulticastPacket_Response) NsBytes() ([]byte, error) {
	p, err := s.Struct.Ptr(0)
	return p.TextBytes(), err
}

func (s MulticastPacket_Response) SetNs(v string) error {
	return s.Struct.SetText(0, v)
}

func (s MulticastPacket_Response) SignedEnvelope() ([]byte, error) {
	p, err := s.Struct.Ptr(1)
	return []byte(p.Data()), err
}

func (s MulticastPacket_Response) HasSignedEnvelope() bool {
	return s.Struct.HasPtr(1)
}

func (s MulticastPacket_Response) SetSignedEnvelope(v []byte) error {
	return s.Struct.SetData(1, v)
}

// MulticastPacket_Response_List is a list of MulticastPacket_Response.
type MulticastPacket_Response_List struct{ capnp.List }

// NewMulticastPacket_Response creates a new list of MulticastPacket_Response.
func NewMulticastPacket_Response_List(s *capnp.Segment, sz int32) (MulticastPacket_Response_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 0, PointerCount: 2}, sz)
	return MulticastPacket_Response_List{l}, err
}

func (s MulticastPacket_Response_List) At(i int) MulticastPacket_Response {
	return MulticastPacket_Response{s.List.Struct(i)}
}

func (s MulticastPacket_Response_List) Set(i int, v MulticastPacket_Response) error {
	return s.List.SetStruct(i, v.Struct)
}

func (s MulticastPacket_Response_List) String() string {
	str, _ := text.MarshalList(0xb998800b6731ad22, s.List)
	return str
}

// MulticastPacket_Response_Future is a wrapper for a MulticastPacket_Response promised by a client call.
type MulticastPacket_Response_Future struct{ *capnp.Future }

func (p MulticastPacket_Response_Future) Struct() (MulticastPacket_Response, error) {
	s, err := p.Future.Struct()
	return MulticastPacket_Response{s}, err
}

const schema_fa005a3c690f4a62 = "x\xda\\\xcf\xb1J+A\x14\xc6\xf1\xef\x9bI\xee^" +
	"H\x82YV\x88h!\x04\x1b\xc1\x04c\xa3\x88\xa0M" +
	"\x9a\x80\x90\xb1S\xb0\xd8\xc4!,\x86\xd953k\xd0" +
	"\xca\x97\x10|\x09\x1f\xc0\xca\xf7\xf0-\xecD\xd0\x91\xa8" +
	"d\x83\xed9p\xfe\xbfS\xbf?\x12\x9drC\x00j" +
	"\xa5\xfc\xcf7\x1f;\xa3\xca\xdd\xc3\x13\xc25\xfa\xe6\xee" +
	" \x96\xee\xf4\x0de\x11\x00\x9d\xd7&#2\x00\xc2\x8f" +
	")\x16\xb6\xaaB>\x0fzK\xc9\xc1\xd9{\x97\x81\x00" +
	"\xa2s\xbeD\x09\x1b@\x94s\x8a\x96\x1f\xa4\xa9k\x0f" +
	"\xe3L\x9al\xff8\x1f\xbbd\x18[\xd7\x8f\x87\x97\xda" +
	"\xb5O\xb4\xcd\xd2\xc0X\xdd'\xd5\x7fY\x02J\x04\xc2" +
	"\xcdU@mH\xaam\xc1\x90\\\xe6l\xd8\xba\x05\xd4" +
	"\x96\xa4\xda\x13\x94\xc6\xb2\x0a\xc1*\xe8m22\xfa\xa2" +
	"kpx\xad\xc7i\xa6Y\x83`\x0d\x9c\x87\xc5\xdf\xb0" +
	"\xd4N\x95\xc8\x85\x87\xd9\xf3\xdf\x14c5\x80\x99\xa4\xea" +
	"\xfd\x0fe\xa7\xa0\xd4\xf8\xe9\x7f-\xbd\xc2\xb2~\x95\xeb" +
	"\xc9\xcd\x9c3)\xee\xb0^$@\xd6\xc1\xaf\x00\x00\x00" +
	"\xff\xff\xeb\xc6Z&"

func init() {
	schemas.Register(schema_fa005a3c690f4a62,
		0xb998800b6731ad22,
		0xf859740361623722)
}
