// Code generated by capnpc-go. DO NOT EDIT.

package pulse

import (
	capnp "capnproto.org/go/capnp/v3"
	text "capnproto.org/go/capnp/v3/encoding/text"
	schemas "capnproto.org/go/capnp/v3/schemas"
	strconv "strconv"
)

type Announcement struct{ capnp.Struct }
type Announcement_Which uint16

const (
	Announcement_Which_heartbeat Announcement_Which = 0
	Announcement_Which_join      Announcement_Which = 1
	Announcement_Which_leave     Announcement_Which = 2
)

func (w Announcement_Which) String() string {
	const s = "heartbeatjoinleave"
	switch w {
	case Announcement_Which_heartbeat:
		return s[0:9]
	case Announcement_Which_join:
		return s[9:13]
	case Announcement_Which_leave:
		return s[13:18]

	}
	return "Announcement_Which(" + strconv.FormatUint(uint64(w), 10) + ")"
}

// Announcement_TypeID is the unique identifier for the type Announcement.
const Announcement_TypeID = 0xffb4e20298d498a4

func NewAnnouncement(s *capnp.Segment) (Announcement, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return Announcement{st}, err
}

func NewRootAnnouncement(s *capnp.Segment) (Announcement, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1})
	return Announcement{st}, err
}

func ReadRootAnnouncement(msg *capnp.Message) (Announcement, error) {
	root, err := msg.Root()
	return Announcement{root.Struct()}, err
}

func (s Announcement) String() string {
	str, _ := text.Marshal(0xffb4e20298d498a4, s.Struct)
	return str
}

func (s Announcement) Which() Announcement_Which {
	return Announcement_Which(s.Struct.Uint16(0))
}
func (s Announcement) Heartbeat() (Announcement_Heartbeat, error) {
	if s.Struct.Uint16(0) != 0 {
		panic("Which() != heartbeat")
	}
	p, err := s.Struct.Ptr(0)
	return Announcement_Heartbeat{Struct: p.Struct()}, err
}

func (s Announcement) HasHeartbeat() bool {
	if s.Struct.Uint16(0) != 0 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s Announcement) SetHeartbeat(v Announcement_Heartbeat) error {
	s.Struct.SetUint16(0, 0)
	return s.Struct.SetPtr(0, v.Struct.ToPtr())
}

// NewHeartbeat sets the heartbeat field to a newly
// allocated Announcement_Heartbeat struct, preferring placement in s's segment.
func (s Announcement) NewHeartbeat() (Announcement_Heartbeat, error) {
	s.Struct.SetUint16(0, 0)
	ss, err := NewAnnouncement_Heartbeat(s.Struct.Segment())
	if err != nil {
		return Announcement_Heartbeat{}, err
	}
	err = s.Struct.SetPtr(0, ss.Struct.ToPtr())
	return ss, err
}

func (s Announcement) Join() (string, error) {
	if s.Struct.Uint16(0) != 1 {
		panic("Which() != join")
	}
	p, err := s.Struct.Ptr(0)
	return p.Text(), err
}

func (s Announcement) HasJoin() bool {
	if s.Struct.Uint16(0) != 1 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s Announcement) JoinBytes() ([]byte, error) {
	p, err := s.Struct.Ptr(0)
	return p.TextBytes(), err
}

func (s Announcement) SetJoin(v string) error {
	s.Struct.SetUint16(0, 1)
	return s.Struct.SetText(0, v)
}

func (s Announcement) Leave() (string, error) {
	if s.Struct.Uint16(0) != 2 {
		panic("Which() != leave")
	}
	p, err := s.Struct.Ptr(0)
	return p.Text(), err
}

func (s Announcement) HasLeave() bool {
	if s.Struct.Uint16(0) != 2 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s Announcement) LeaveBytes() ([]byte, error) {
	p, err := s.Struct.Ptr(0)
	return p.TextBytes(), err
}

func (s Announcement) SetLeave(v string) error {
	s.Struct.SetUint16(0, 2)
	return s.Struct.SetText(0, v)
}

// Announcement_List is a list of Announcement.
type Announcement_List struct{ capnp.List }

// NewAnnouncement creates a new list of Announcement.
func NewAnnouncement_List(s *capnp.Segment, sz int32) (Announcement_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 8, PointerCount: 1}, sz)
	return Announcement_List{l}, err
}

func (s Announcement_List) At(i int) Announcement { return Announcement{s.List.Struct(i)} }

func (s Announcement_List) Set(i int, v Announcement) error { return s.List.SetStruct(i, v.Struct) }

func (s Announcement_List) String() string {
	str, _ := text.MarshalList(0xffb4e20298d498a4, s.List)
	return str
}

// Announcement_Future is a wrapper for a Announcement promised by a client call.
type Announcement_Future struct{ *capnp.Future }

func (p Announcement_Future) Struct() (Announcement, error) {
	s, err := p.Future.Struct()
	return Announcement{s}, err
}

func (p Announcement_Future) Heartbeat() Announcement_Heartbeat_Future {
	return Announcement_Heartbeat_Future{Future: p.Future.Field(0, nil)}
}

type Announcement_Heartbeat struct{ capnp.Struct }
type Announcement_Heartbeat_record Announcement_Heartbeat
type Announcement_Heartbeat_record_Which uint16

const (
	Announcement_Heartbeat_record_Which_none    Announcement_Heartbeat_record_Which = 0
	Announcement_Heartbeat_record_Which_text    Announcement_Heartbeat_record_Which = 1
	Announcement_Heartbeat_record_Which_binary  Announcement_Heartbeat_record_Which = 2
	Announcement_Heartbeat_record_Which_pointer Announcement_Heartbeat_record_Which = 3
)

func (w Announcement_Heartbeat_record_Which) String() string {
	const s = "nonetextbinarypointer"
	switch w {
	case Announcement_Heartbeat_record_Which_none:
		return s[0:4]
	case Announcement_Heartbeat_record_Which_text:
		return s[4:8]
	case Announcement_Heartbeat_record_Which_binary:
		return s[8:14]
	case Announcement_Heartbeat_record_Which_pointer:
		return s[14:21]

	}
	return "Announcement_Heartbeat_record_Which(" + strconv.FormatUint(uint64(w), 10) + ")"
}

// Announcement_Heartbeat_TypeID is the unique identifier for the type Announcement_Heartbeat.
const Announcement_Heartbeat_TypeID = 0xa3743b0938b604e5

func NewAnnouncement_Heartbeat(s *capnp.Segment) (Announcement_Heartbeat, error) {
	st, err := capnp.NewStruct(s, capnp.ObjectSize{DataSize: 16, PointerCount: 1})
	return Announcement_Heartbeat{st}, err
}

func NewRootAnnouncement_Heartbeat(s *capnp.Segment) (Announcement_Heartbeat, error) {
	st, err := capnp.NewRootStruct(s, capnp.ObjectSize{DataSize: 16, PointerCount: 1})
	return Announcement_Heartbeat{st}, err
}

func ReadRootAnnouncement_Heartbeat(msg *capnp.Message) (Announcement_Heartbeat, error) {
	root, err := msg.Root()
	return Announcement_Heartbeat{root.Struct()}, err
}

func (s Announcement_Heartbeat) String() string {
	str, _ := text.Marshal(0xa3743b0938b604e5, s.Struct)
	return str
}

func (s Announcement_Heartbeat) Ttl() int64 {
	return int64(s.Struct.Uint64(0))
}

func (s Announcement_Heartbeat) SetTtl(v int64) {
	s.Struct.SetUint64(0, uint64(v))
}

func (s Announcement_Heartbeat) Record() Announcement_Heartbeat_record {
	return Announcement_Heartbeat_record(s)
}

func (s Announcement_Heartbeat_record) Which() Announcement_Heartbeat_record_Which {
	return Announcement_Heartbeat_record_Which(s.Struct.Uint16(8))
}
func (s Announcement_Heartbeat_record) SetNone() {
	s.Struct.SetUint16(8, 0)

}

func (s Announcement_Heartbeat_record) Text() (string, error) {
	if s.Struct.Uint16(8) != 1 {
		panic("Which() != text")
	}
	p, err := s.Struct.Ptr(0)
	return p.Text(), err
}

func (s Announcement_Heartbeat_record) HasText() bool {
	if s.Struct.Uint16(8) != 1 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s Announcement_Heartbeat_record) TextBytes() ([]byte, error) {
	p, err := s.Struct.Ptr(0)
	return p.TextBytes(), err
}

func (s Announcement_Heartbeat_record) SetText(v string) error {
	s.Struct.SetUint16(8, 1)
	return s.Struct.SetText(0, v)
}

func (s Announcement_Heartbeat_record) Binary() ([]byte, error) {
	if s.Struct.Uint16(8) != 2 {
		panic("Which() != binary")
	}
	p, err := s.Struct.Ptr(0)
	return []byte(p.Data()), err
}

func (s Announcement_Heartbeat_record) HasBinary() bool {
	if s.Struct.Uint16(8) != 2 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s Announcement_Heartbeat_record) SetBinary(v []byte) error {
	s.Struct.SetUint16(8, 2)
	return s.Struct.SetData(0, v)
}

func (s Announcement_Heartbeat_record) Pointer() (capnp.Ptr, error) {
	if s.Struct.Uint16(8) != 3 {
		panic("Which() != pointer")
	}
	return s.Struct.Ptr(0)
}

func (s Announcement_Heartbeat_record) HasPointer() bool {
	if s.Struct.Uint16(8) != 3 {
		return false
	}
	return s.Struct.HasPtr(0)
}

func (s Announcement_Heartbeat_record) SetPointer(v capnp.Ptr) error {
	s.Struct.SetUint16(8, 3)
	return s.Struct.SetPtr(0, v)
}

// Announcement_Heartbeat_List is a list of Announcement_Heartbeat.
type Announcement_Heartbeat_List struct{ capnp.List }

// NewAnnouncement_Heartbeat creates a new list of Announcement_Heartbeat.
func NewAnnouncement_Heartbeat_List(s *capnp.Segment, sz int32) (Announcement_Heartbeat_List, error) {
	l, err := capnp.NewCompositeList(s, capnp.ObjectSize{DataSize: 16, PointerCount: 1}, sz)
	return Announcement_Heartbeat_List{l}, err
}

func (s Announcement_Heartbeat_List) At(i int) Announcement_Heartbeat {
	return Announcement_Heartbeat{s.List.Struct(i)}
}

func (s Announcement_Heartbeat_List) Set(i int, v Announcement_Heartbeat) error {
	return s.List.SetStruct(i, v.Struct)
}

func (s Announcement_Heartbeat_List) String() string {
	str, _ := text.MarshalList(0xa3743b0938b604e5, s.List)
	return str
}

// Announcement_Heartbeat_Future is a wrapper for a Announcement_Heartbeat promised by a client call.
type Announcement_Heartbeat_Future struct{ *capnp.Future }

func (p Announcement_Heartbeat_Future) Struct() (Announcement_Heartbeat, error) {
	s, err := p.Future.Struct()
	return Announcement_Heartbeat{s}, err
}

func (p Announcement_Heartbeat_Future) Record() Announcement_Heartbeat_record_Future {
	return Announcement_Heartbeat_record_Future{p.Future}
}

// Announcement_Heartbeat_record_Future is a wrapper for a Announcement_Heartbeat_record promised by a client call.
type Announcement_Heartbeat_record_Future struct{ *capnp.Future }

func (p Announcement_Heartbeat_record_Future) Struct() (Announcement_Heartbeat_record, error) {
	s, err := p.Future.Struct()
	return Announcement_Heartbeat_record{s}, err
}

func (p Announcement_Heartbeat_record_Future) Pointer() *capnp.Future {
	return p.Future.Field(0, nil)
}

const schema_c2974e3dc137fcee = "x\xdat\x91Ok\x13A\x18\xc6\x9f\xe7\x9dM\xa3\x90" +
	"\x98l7\xe0AE\x10=\xb4\x90\xa0\x16Q\"R\xff" +
	"\xb4PA\xa5o\xb1\xe0\xc1\x83\x9bd \x91t6\xac" +
	"\x13\xff\x9c\xfc\x18\x16<\xaag/\xc5\xbb~\x06O\x1e" +
	"\x05\x11<\xf8\x09\xaa#[\xd8.\x08\x9d\xd3\xcc\x0b\xf3" +
	"\xfc\x9e\xdfL\xfb\xd7M\xb9T\xcb\x04\xd0\xd3\xb5\x85\xf0" +
	"#\xfat\xed\xf8u\xff\x0ez\x8a\x12\xde\xef~\xdd\x95" +
	"\xef{\x015\xd6\x81\x953\\f\xd2-\xb6\xc9\x12_" +
	"\x80\xe1J\xe3\xed\x9d5\xf3a\x0fz\x81R\xdd\xddf" +
	"\x9d\x11\xa3\x95\x09\x07\x04\x939\x7f\x82U\x96\x9e \xc3" +
	"\xef\xfd\xab\x9fo<x\xf3\x05\xeb\xac\x1b \xb9+\xdf" +
	"\x92m9\x09$\xa9|D7\x0c\xa7\xf3g\xde\xe6=" +
	"3Lgn\xd6\xbf\xe5\\6wC\xbbc\x9d\xefm" +
	"\xd84\xf7\xad\x81M\xfd&\xa9\xc7L\x04D\x04\xe2\xa5" +
	"s\x80\x9e7\xd4\x8b\xc2bU\xf5\xe2n\x1fR\xf7~" +
	"\xca\x1a\x845p5\xb7\xc3,\x1f\x1db\xa2\xa30\x05" +
	"\xa5\x97\xdba=\xcbG\xda6Q#\x84\x0e\x0bX\xba" +
	"\x0c\xe8cC\x1d\x0b\x9b\xfc\x1b:\x14 \xb6\xc5\xf4\x89" +
	"\xa1N\x85M\xf9\x13:4@<\xe9\x03:2\xd4\x99" +
	"\xb0i\xf6C\x87\x11\x10\xef\xdc\x06tl\xa8^\xd8r" +
	"\x99\xb3Xhy\xfb\xd2\xb3\x01a\x03\\\x1dL\\\x9a" +
	"\xbfb\x13\xc2&\xf8z\x96M\x9c\xb79\x17!\\\x04" +
	"\x0f\xbb\xcb\xff\xdd\x8d\xf3\x1a\x91\xd5\x7f\xc4\xdc\x0a\xa5\x0d" +
	"\xe8\xb5q\xe0q\xf0f\xeb[\x80\xae\x19\xeaf\xa9Q" +
	"L\xef\x17\x1a\x1b\x86\xfa\xb0\xd4(\xe4\xf42\xa0\xf7\x0c" +
	"\xf5\x910\x8c\xab<\xb6+\x14\xc86\xd8z\x9aM\\" +
	"\xe9qvj\xd3\xe7\xb6<\xfd\x0b\x00\x00\xff\xff\x19\xaa" +
	"\x9fg"

func init() {
	schemas.Register(schema_c2974e3dc137fcee,
		0xa3743b0938b604e5,
		0xb4a50344439b0c35,
		0xffb4e20298d498a4)
}
