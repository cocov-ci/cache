// Code generated by MockGen. DO NOT EDIT.
// Source: redis/redis.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"
	time "time"

	redis "github.com/cocov-ci/cache/redis"
	gomock "github.com/golang/mock/gomock"
	leader "github.com/heyvito/go-leader/leader"
)

// MockRedisClient is a mock of Client interface.
type MockRedisClient struct {
	ctrl     *gomock.Controller
	recorder *MockRedisClientMockRecorder
}

// MockRedisClientMockRecorder is the mock recorder for MockRedisClient.
type MockRedisClientMockRecorder struct {
	mock *MockRedisClient
}

// NewMockRedisClient creates a new mock instance.
func NewMockRedisClient(ctrl *gomock.Controller) *MockRedisClient {
	mock := &MockRedisClient{ctrl: ctrl}
	mock.recorder = &MockRedisClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockRedisClient) EXPECT() *MockRedisClientMockRecorder {
	return m.recorder
}

// Locking mocks base method.
func (m *MockRedisClient) Locking(id string, timeout time.Duration, fn func() error) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Locking", id, timeout, fn)
	ret0, _ := ret[0].(error)
	return ret0
}

// Locking indicates an expected call of Locking.
func (mr *MockRedisClientMockRecorder) Locking(id, timeout, fn interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Locking", reflect.TypeOf((*MockRedisClient)(nil).Locking), id, timeout, fn)
}

// MakeLeader mocks base method.
func (m *MockRedisClient) MakeLeader(opts leader.Opts) (leader.Leader, <-chan time.Time, <-chan time.Time, <-chan error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MakeLeader", opts)
	ret0, _ := ret[0].(leader.Leader)
	ret1, _ := ret[1].(<-chan time.Time)
	ret2, _ := ret[2].(<-chan time.Time)
	ret3, _ := ret[3].(<-chan error)
	return ret0, ret1, ret2, ret3
}

// MakeLeader indicates an expected call of MakeLeader.
func (mr *MockRedisClientMockRecorder) MakeLeader(opts interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MakeLeader", reflect.TypeOf((*MockRedisClient)(nil).MakeLeader), opts)
}

// NextHousekeepingTask mocks base method.
func (m *MockRedisClient) NextHousekeepingTask() (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NextHousekeepingTask")
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// NextHousekeepingTask indicates an expected call of NextHousekeepingTask.
func (mr *MockRedisClientMockRecorder) NextHousekeepingTask() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NextHousekeepingTask", reflect.TypeOf((*MockRedisClient)(nil).NextHousekeepingTask))
}

// RepoDataFromJID mocks base method.
func (m *MockRedisClient) RepoDataFromJID(jid string) (*redis.RepoIdentifier, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RepoDataFromJID", jid)
	ret0, _ := ret[0].(*redis.RepoIdentifier)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RepoDataFromJID indicates an expected call of RepoDataFromJID.
func (mr *MockRedisClientMockRecorder) RepoDataFromJID(jid interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RepoDataFromJID", reflect.TypeOf((*MockRedisClient)(nil).RepoDataFromJID), jid)
}
