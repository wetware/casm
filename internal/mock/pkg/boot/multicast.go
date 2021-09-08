// Code generated by MockGen. DO NOT EDIT.
// Source: multicast.go

// Package mock_boot is a generated GoMock package.
package mock_boot

import (
	context "context"
	reflect "reflect"

	capnp "capnproto.org/go/capnp/v3"
	gomock "github.com/golang/mock/gomock"
	boot "github.com/wetware/casm/pkg/boot"
)

// MockTransport is a mock of Transport interface.
type MockTransport struct {
	ctrl     *gomock.Controller
	recorder *MockTransportMockRecorder
}

// MockTransportMockRecorder is the mock recorder for MockTransport.
type MockTransportMockRecorder struct {
	mock *MockTransport
}

// NewMockTransport creates a new mock instance.
func NewMockTransport(ctrl *gomock.Controller) *MockTransport {
	mock := &MockTransport{ctrl: ctrl}
	mock.recorder = &MockTransportMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTransport) EXPECT() *MockTransportMockRecorder {
	return m.recorder
}

// Dial mocks base method.
func (m *MockTransport) Dial() (boot.Scatterer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Dial")
	ret0, _ := ret[0].(boot.Scatterer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Dial indicates an expected call of Dial.
func (mr *MockTransportMockRecorder) Dial() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Dial", reflect.TypeOf((*MockTransport)(nil).Dial))
}

// Listen mocks base method.
func (m *MockTransport) Listen() (boot.Gatherer, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Listen")
	ret0, _ := ret[0].(boot.Gatherer)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Listen indicates an expected call of Listen.
func (mr *MockTransportMockRecorder) Listen() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Listen", reflect.TypeOf((*MockTransport)(nil).Listen))
}

// MockScatterer is a mock of Scatterer interface.
type MockScatterer struct {
	ctrl     *gomock.Controller
	recorder *MockScattererMockRecorder
}

// MockScattererMockRecorder is the mock recorder for MockScatterer.
type MockScattererMockRecorder struct {
	mock *MockScatterer
}

// NewMockScatterer creates a new mock instance.
func NewMockScatterer(ctrl *gomock.Controller) *MockScatterer {
	mock := &MockScatterer{ctrl: ctrl}
	mock.recorder = &MockScattererMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockScatterer) EXPECT() *MockScattererMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockScatterer) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockScattererMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockScatterer)(nil).Close))
}

// Scatter mocks base method.
func (m *MockScatterer) Scatter(arg0 context.Context, arg1 *capnp.Message) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Scatter", arg0, arg1)
	ret0, _ := ret[0].(error)
	return ret0
}

// Scatter indicates an expected call of Scatter.
func (mr *MockScattererMockRecorder) Scatter(arg0, arg1 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Scatter", reflect.TypeOf((*MockScatterer)(nil).Scatter), arg0, arg1)
}

// MockGatherer is a mock of Gatherer interface.
type MockGatherer struct {
	ctrl     *gomock.Controller
	recorder *MockGathererMockRecorder
}

// MockGathererMockRecorder is the mock recorder for MockGatherer.
type MockGathererMockRecorder struct {
	mock *MockGatherer
}

// NewMockGatherer creates a new mock instance.
func NewMockGatherer(ctrl *gomock.Controller) *MockGatherer {
	mock := &MockGatherer{ctrl: ctrl}
	mock.recorder = &MockGathererMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockGatherer) EXPECT() *MockGathererMockRecorder {
	return m.recorder
}

// Close mocks base method.
func (m *MockGatherer) Close() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Close")
	ret0, _ := ret[0].(error)
	return ret0
}

// Close indicates an expected call of Close.
func (mr *MockGathererMockRecorder) Close() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Close", reflect.TypeOf((*MockGatherer)(nil).Close))
}

// Gather mocks base method.
func (m *MockGatherer) Gather(arg0 context.Context) (*capnp.Message, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Gather", arg0)
	ret0, _ := ret[0].(*capnp.Message)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Gather indicates an expected call of Gather.
func (mr *MockGathererMockRecorder) Gather(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Gather", reflect.TypeOf((*MockGatherer)(nil).Gather), arg0)
}
