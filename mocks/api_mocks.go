// Code generated by MockGen. DO NOT EDIT.
// Source: api/api.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	api "github.com/cocov-ci/cache/api"
	gomock "go.uber.org/mock/gomock"
)

// MockAPIClient is a mock of Client interface.
type MockAPIClient struct {
	ctrl     *gomock.Controller
	recorder *MockAPIClientMockRecorder
}

// MockAPIClientMockRecorder is the mock recorder for MockAPIClient.
type MockAPIClientMockRecorder struct {
	mock *MockAPIClient
}

// NewMockAPIClient creates a new mock instance.
func NewMockAPIClient(ctrl *gomock.Controller) *MockAPIClient {
	mock := &MockAPIClient{ctrl: ctrl}
	mock.recorder = &MockAPIClientMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockAPIClient) EXPECT() *MockAPIClientMockRecorder {
	return m.recorder
}

// DeleteArtifact mocks base method.
func (m *MockAPIClient) DeleteArtifact(arg0 api.DeleteArtifactInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteArtifact", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteArtifact indicates an expected call of DeleteArtifact.
func (mr *MockAPIClientMockRecorder) DeleteArtifact(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteArtifact", reflect.TypeOf((*MockAPIClient)(nil).DeleteArtifact), arg0)
}

// DeleteTool mocks base method.
func (m *MockAPIClient) DeleteTool(input api.DeleteToolInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DeleteTool", input)
	ret0, _ := ret[0].(error)
	return ret0
}

// DeleteTool indicates an expected call of DeleteTool.
func (mr *MockAPIClientMockRecorder) DeleteTool(input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DeleteTool", reflect.TypeOf((*MockAPIClient)(nil).DeleteTool), input)
}

// GetArtifactMeta mocks base method.
func (m *MockAPIClient) GetArtifactMeta(arg0 api.GetArtifactMetaInput) (*api.GetArtifactMetaOutput, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetArtifactMeta", arg0)
	ret0, _ := ret[0].(*api.GetArtifactMetaOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetArtifactMeta indicates an expected call of GetArtifactMeta.
func (mr *MockAPIClientMockRecorder) GetArtifactMeta(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetArtifactMeta", reflect.TypeOf((*MockAPIClient)(nil).GetArtifactMeta), arg0)
}

// GetToolMeta mocks base method.
func (m *MockAPIClient) GetToolMeta(arg0 api.GetToolMetaInput) (*api.GetToolMetaOutput, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetToolMeta", arg0)
	ret0, _ := ret[0].(*api.GetToolMetaOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetToolMeta indicates an expected call of GetToolMeta.
func (mr *MockAPIClientMockRecorder) GetToolMeta(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetToolMeta", reflect.TypeOf((*MockAPIClient)(nil).GetToolMeta), arg0)
}

// Ping mocks base method.
func (m *MockAPIClient) Ping() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Ping")
	ret0, _ := ret[0].(error)
	return ret0
}

// Ping indicates an expected call of Ping.
func (mr *MockAPIClientMockRecorder) Ping() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Ping", reflect.TypeOf((*MockAPIClient)(nil).Ping))
}

// RegisterArtifact mocks base method.
func (m *MockAPIClient) RegisterArtifact(arg0 api.RegisterArtifactInput) (*api.RegisterArtifactOutput, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterArtifact", arg0)
	ret0, _ := ret[0].(*api.RegisterArtifactOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegisterArtifact indicates an expected call of RegisterArtifact.
func (mr *MockAPIClientMockRecorder) RegisterArtifact(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterArtifact", reflect.TypeOf((*MockAPIClient)(nil).RegisterArtifact), arg0)
}

// RegisterTool mocks base method.
func (m *MockAPIClient) RegisterTool(arg0 api.RegisterToolInput) (*api.RegisterToolOutput, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "RegisterTool", arg0)
	ret0, _ := ret[0].(*api.RegisterToolOutput)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// RegisterTool indicates an expected call of RegisterTool.
func (mr *MockAPIClientMockRecorder) RegisterTool(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "RegisterTool", reflect.TypeOf((*MockAPIClient)(nil).RegisterTool), arg0)
}

// TouchArtifact mocks base method.
func (m *MockAPIClient) TouchArtifact(arg0 api.TouchArtifactInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TouchArtifact", arg0)
	ret0, _ := ret[0].(error)
	return ret0
}

// TouchArtifact indicates an expected call of TouchArtifact.
func (mr *MockAPIClientMockRecorder) TouchArtifact(arg0 interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TouchArtifact", reflect.TypeOf((*MockAPIClient)(nil).TouchArtifact), arg0)
}

// TouchTool mocks base method.
func (m *MockAPIClient) TouchTool(input api.TouchToolInput) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "TouchTool", input)
	ret0, _ := ret[0].(error)
	return ret0
}

// TouchTool indicates an expected call of TouchTool.
func (mr *MockAPIClientMockRecorder) TouchTool(input interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "TouchTool", reflect.TypeOf((*MockAPIClient)(nil).TouchTool), input)
}
