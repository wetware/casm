// Code generated by MockGen. DO NOT EDIT.
// Source: pulse.go

// Package mock_pulse is a generated GoMock package.
package mock_pulse

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
	pulse "github.com/wetware/casm/pkg/cluster/pulse"
	routing "github.com/wetware/casm/pkg/cluster/routing"
)

// MockRoutingTable is a mock of RoutingTable interface.
type MockRoutingTable struct {
	ctrl     *gomock.Controller
	recorder *MockRoutingTableMockRecorder
}

// MockRoutingTableMockRecorder is the mock recorder for MockRoutingTable.
type MockRoutingTableMockRecorder struct {
	mock *MockRoutingTable
}

// NewMockRoutingTable creates a new mock instance.
func NewMockRoutingTable(ctrl *gomock.Controller) *MockRoutingTable {
	mock := &MockRoutingTable{ctrl: ctrl}
	mock.recorder = &MockRoutingTableMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRoutingTable) EXPECT() *MockRoutingTableMockRecorder {
	return m.recorder
}

// Upsert mocks base method.
func (m *MockRoutingTable) Upsert(arg0 routing.Record) bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Upsert", arg0)
	ret0, _ := ret[0].(bool)
	return ret0
}

// Upsert indicates an expected call of Upsert.
func (mr *MockRoutingTableMockRecorder) Upsert(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Upsert", reflect.TypeOf((*MockRoutingTable)(nil).Upsert), arg0)
}

// MockPreparer is a mock of Preparer interface.
type MockPreparer struct {
	ctrl     *gomock.Controller
	recorder *MockPreparerMockRecorder
}

// MockPreparerMockRecorder is the mock recorder for MockPreparer.
type MockPreparerMockRecorder struct {
	mock *MockPreparer
}

// NewMockPreparer creates a new mock instance.
func NewMockPreparer(ctrl *gomock.Controller) *MockPreparer {
	mock := &MockPreparer{ctrl: ctrl}
	mock.recorder = &MockPreparerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockPreparer) EXPECT() *MockPreparerMockRecorder {
	return m.recorder
}

// Prepare mocks base method.
func (m *MockPreparer) Prepare(arg0 pulse.Heartbeat) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Prepare", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// Prepare indicates an expected call of Prepare.
func (mr *MockPreparerMockRecorder) Prepare(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Prepare", reflect.TypeOf((*MockPreparer)(nil).Prepare), arg0)
}
