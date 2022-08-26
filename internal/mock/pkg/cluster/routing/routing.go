// Code generated by MockGen. DO NOT EDIT.
// Source: routing.go

// Package mock_routing is a generated GoMock package.
package mock_routing

import (
	reflect "reflect"
	time "time"

	gomock "github.com/golang/mock/gomock"
	peer "github.com/libp2p/go-libp2p/core/peer"
	routing "github.com/wetware/casm/pkg/cluster/routing"
)

// MockRecord is a mock of Record interface.
type MockRecord struct {
	ctrl     *gomock.Controller
	recorder *MockRecordMockRecorder
}

// MockRecordMockRecorder is the mock recorder for MockRecord.
type MockRecordMockRecorder struct {
	mock *MockRecord
}

// NewMockRecord creates a new mock instance.
func NewMockRecord(ctrl *gomock.Controller) *MockRecord {
	mock := &MockRecord{ctrl: ctrl}
	mock.recorder = &MockRecordMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRecord) EXPECT() *MockRecordMockRecorder {
	return m.recorder
}

// Host mocks base method.
func (m *MockRecord) Host() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Host")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Host indicates an expected call of Host.
func (mr *MockRecordMockRecorder) Host() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Host", reflect.TypeOf((*MockRecord)(nil).Host))
}

// Instance mocks base method.
func (m *MockRecord) Instance() uint32 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Instance")
	ret0, _ := ret[0].(uint32)
	return ret0
}

// Instance indicates an expected call of Instance.
func (mr *MockRecordMockRecorder) Instance() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Instance", reflect.TypeOf((*MockRecord)(nil).Instance))
}

// Meta mocks base method.
func (m *MockRecord) Meta() (routing.Meta, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Meta")
	ret0, _ := ret[0].(routing.Meta)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Meta indicates an expected call of Meta.
func (mr *MockRecordMockRecorder) Meta() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Meta", reflect.TypeOf((*MockRecord)(nil).Meta))
}

// Peer mocks base method.
func (m *MockRecord) Peer() peer.ID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Peer")
	ret0, _ := ret[0].(peer.ID)
	return ret0
}

// Peer indicates an expected call of Peer.
func (mr *MockRecordMockRecorder) Peer() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Peer", reflect.TypeOf((*MockRecord)(nil).Peer))
}

// Seq mocks base method.
func (m *MockRecord) Seq() uint64 {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Seq")
	ret0, _ := ret[0].(uint64)
	return ret0
}

// Seq indicates an expected call of Seq.
func (mr *MockRecordMockRecorder) Seq() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Seq", reflect.TypeOf((*MockRecord)(nil).Seq))
}

// TTL mocks base method.
func (m *MockRecord) TTL() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TTL")
	ret0, _ := ret[0].(time.Duration)
	return ret0
}

// TTL indicates an expected call of TTL.
func (mr *MockRecordMockRecorder) TTL() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TTL", reflect.TypeOf((*MockRecord)(nil).TTL))
}

// MockSnapshot is a mock of Snapshot interface.
type MockSnapshot struct {
	ctrl     *gomock.Controller
	recorder *MockSnapshotMockRecorder
}

// MockSnapshotMockRecorder is the mock recorder for MockSnapshot.
type MockSnapshotMockRecorder struct {
	mock *MockSnapshot
}

// NewMockSnapshot creates a new mock instance.
func NewMockSnapshot(ctrl *gomock.Controller) *MockSnapshot {
	mock := &MockSnapshot{ctrl: ctrl}
	mock.recorder = &MockSnapshotMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSnapshot) EXPECT() *MockSnapshotMockRecorder {
	return m.recorder
}

// Get mocks base method.
func (m *MockSnapshot) Get(arg0 routing.Index) (routing.Iterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", arg0)
	ret0, _ := ret[0].(routing.Iterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockSnapshotMockRecorder) Get(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockSnapshot)(nil).Get), arg0)
}

// GetReverse mocks base method.
func (m *MockSnapshot) GetReverse(arg0 routing.Index) (routing.Iterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetReverse", arg0)
	ret0, _ := ret[0].(routing.Iterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetReverse indicates an expected call of GetReverse.
func (mr *MockSnapshotMockRecorder) GetReverse(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetReverse", reflect.TypeOf((*MockSnapshot)(nil).GetReverse), arg0)
}

// LowerBound mocks base method.
func (m *MockSnapshot) LowerBound(arg0 routing.Index) (routing.Iterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "LowerBound", arg0)
	ret0, _ := ret[0].(routing.Iterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// LowerBound indicates an expected call of LowerBound.
func (mr *MockSnapshotMockRecorder) LowerBound(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "LowerBound", reflect.TypeOf((*MockSnapshot)(nil).LowerBound), arg0)
}

// ReverseLowerBound mocks base method.
func (m *MockSnapshot) ReverseLowerBound(arg0 routing.Index) (routing.Iterator, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "ReverseLowerBound", arg0)
	ret0, _ := ret[0].(routing.Iterator)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// ReverseLowerBound indicates an expected call of ReverseLowerBound.
func (mr *MockSnapshotMockRecorder) ReverseLowerBound(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "ReverseLowerBound", reflect.TypeOf((*MockSnapshot)(nil).ReverseLowerBound), arg0)
}

// MockIndex is a mock of Index interface.
type MockIndex struct {
	ctrl     *gomock.Controller
	recorder *MockIndexMockRecorder
}

// MockIndexMockRecorder is the mock recorder for MockIndex.
type MockIndexMockRecorder struct {
	mock *MockIndex
}

// NewMockIndex creates a new mock instance.
func NewMockIndex(ctrl *gomock.Controller) *MockIndex {
	mock := &MockIndex{ctrl: ctrl}
	mock.recorder = &MockIndexMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIndex) EXPECT() *MockIndexMockRecorder {
	return m.recorder
}

// Key mocks base method.
func (m *MockIndex) Key() routing.IndexKey {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Key")
	ret0, _ := ret[0].(routing.IndexKey)
	return ret0
}

// Key indicates an expected call of Key.
func (mr *MockIndexMockRecorder) Key() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Key", reflect.TypeOf((*MockIndex)(nil).Key))
}

// Match mocks base method.
func (m *MockIndex) Match(arg0 routing.Record) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Match", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Match indicates an expected call of Match.
func (mr *MockIndexMockRecorder) Match(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Match", reflect.TypeOf((*MockIndex)(nil).Match), arg0)
}

// String mocks base method.
func (m *MockIndex) String() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "String")
	ret0, _ := ret[0].(string)
	return ret0
}

// String indicates an expected call of String.
func (mr *MockIndexMockRecorder) String() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "String", reflect.TypeOf((*MockIndex)(nil).String))
}

// MockPeerIndex is a mock of PeerIndex interface.
type MockPeerIndex struct {
	ctrl     *gomock.Controller
	recorder *MockPeerIndexMockRecorder
}

// MockPeerIndexMockRecorder is the mock recorder for MockPeerIndex.
type MockPeerIndexMockRecorder struct {
	mock *MockPeerIndex
}

// NewMockPeerIndex creates a new mock instance.
func NewMockPeerIndex(ctrl *gomock.Controller) *MockPeerIndex {
	mock := &MockPeerIndex{ctrl: ctrl}
	mock.recorder = &MockPeerIndexMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPeerIndex) EXPECT() *MockPeerIndexMockRecorder {
	return m.recorder
}

// PeerBytes mocks base method.
func (m *MockPeerIndex) PeerBytes() ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "PeerBytes")
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// PeerBytes indicates an expected call of PeerBytes.
func (mr *MockPeerIndexMockRecorder) PeerBytes() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "PeerBytes", reflect.TypeOf((*MockPeerIndex)(nil).PeerBytes))
}

// MockHostIndex is a mock of HostIndex interface.
type MockHostIndex struct {
	ctrl     *gomock.Controller
	recorder *MockHostIndexMockRecorder
}

// MockHostIndexMockRecorder is the mock recorder for MockHostIndex.
type MockHostIndexMockRecorder struct {
	mock *MockHostIndex
}

// NewMockHostIndex creates a new mock instance.
func NewMockHostIndex(ctrl *gomock.Controller) *MockHostIndex {
	mock := &MockHostIndex{ctrl: ctrl}
	mock.recorder = &MockHostIndexMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockHostIndex) EXPECT() *MockHostIndexMockRecorder {
	return m.recorder
}

// HostBytes mocks base method.
func (m *MockHostIndex) HostBytes() ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "HostBytes")
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// HostBytes indicates an expected call of HostBytes.
func (mr *MockHostIndexMockRecorder) HostBytes() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "HostBytes", reflect.TypeOf((*MockHostIndex)(nil).HostBytes))
}

// MockMetaIndex is a mock of MetaIndex interface.
type MockMetaIndex struct {
	ctrl     *gomock.Controller
	recorder *MockMetaIndexMockRecorder
}

// MockMetaIndexMockRecorder is the mock recorder for MockMetaIndex.
type MockMetaIndexMockRecorder struct {
	mock *MockMetaIndex
}

// NewMockMetaIndex creates a new mock instance.
func NewMockMetaIndex(ctrl *gomock.Controller) *MockMetaIndex {
	mock := &MockMetaIndex{ctrl: ctrl}
	mock.recorder = &MockMetaIndexMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockMetaIndex) EXPECT() *MockMetaIndexMockRecorder {
	return m.recorder
}

// MetaBytes mocks base method.
func (m *MockMetaIndex) MetaBytes() ([][]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MetaBytes")
	ret0, _ := ret[0].([][]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// MetaBytes indicates an expected call of MetaBytes.
func (mr *MockMetaIndexMockRecorder) MetaBytes() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MetaBytes", reflect.TypeOf((*MockMetaIndex)(nil).MetaBytes))
}

// MockIterator is a mock of Iterator interface.
type MockIterator struct {
	ctrl     *gomock.Controller
	recorder *MockIteratorMockRecorder
}

// MockIteratorMockRecorder is the mock recorder for MockIterator.
type MockIteratorMockRecorder struct {
	mock *MockIterator
}

// NewMockIterator creates a new mock instance.
func NewMockIterator(ctrl *gomock.Controller) *MockIterator {
	mock := &MockIterator{ctrl: ctrl}
	mock.recorder = &MockIteratorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIterator) EXPECT() *MockIteratorMockRecorder {
	return m.recorder
}

// Next mocks base method.
func (m *MockIterator) Next() routing.Record {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Next")
	ret0, _ := ret[0].(routing.Record)
	return ret0
}

// Next indicates an expected call of Next.
func (mr *MockIteratorMockRecorder) Next() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Next", reflect.TypeOf((*MockIterator)(nil).Next))
}
