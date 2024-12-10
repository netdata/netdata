// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logind

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/coreos/go-systemd/v22/login1"
	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"default config": {
			wantFail: false,
			config:   New().Config,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.Equal(t, len(charts), len(*New().Charts()))
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		wantClose bool
		prepare   func(l *Collector)
	}{
		"after New": {
			wantClose: false,
			prepare:   func(l *Collector) {},
		},
		"after Init": {
			wantClose: false,
			prepare:   func(l *Collector) { _ = l.Init(context.Background()) },
		},
		"after Check": {
			wantClose: true,
			prepare:   func(l *Collector) { _ = l.Init(context.Background()); _ = l.Check(context.Background()) },
		},
		"after Collect": {
			wantClose: true,
			prepare:   func(l *Collector) { _ = l.Init(context.Background()); l.Collect(context.Background()) },
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			m := prepareConnOK()
			collr.newLogindConn = func(Config) (logindConnection, error) { return m, nil }
			test.prepare(collr)

			require.NotPanics(t, func() { collr.Cleanup(context.Background()) })

			if test.wantClose {
				assert.True(t, m.closeCalled)
			} else {
				assert.False(t, m.closeCalled)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func() *mockConn
	}{
		"success when response contains sessions and users": {
			wantFail: false,
			prepare:  prepareConnOK,
		},
		"success when response does not contain sessions and users": {
			wantFail: false,
			prepare:  prepareConnOKNoSessionsNoUsers,
		},
		"fail when error on list sessions": {
			wantFail: true,
			prepare:  prepareConnErrOnListSessions,
		},
		"fail when error on get session properties": {
			wantFail: true,
			prepare:  prepareConnErrOnGetSessionProperties,
		},
		"fail when error on list users": {
			wantFail: true,
			prepare:  prepareConnErrOnListUsers,
		},
		"fail when error on get user property": {
			wantFail: true,
			prepare:  prepareConnErrOnGetUserProperty,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			require.NoError(t, collr.Init(context.Background()))
			collr.conn = test.prepare()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare  func() *mockConn
		expected map[string]int64
	}{
		"success when response contains sessions and users": {
			prepare: prepareConnOK,
			expected: map[string]int64{
				"sessions_local":          3,
				"sessions_remote":         0,
				"sessions_state_active":   0,
				"sessions_state_closing":  0,
				"sessions_state_online":   3,
				"sessions_type_console":   3,
				"sessions_type_graphical": 0,
				"sessions_type_other":     0,
				"users_state_active":      3,
				"users_state_closing":     0,
				"users_state_lingering":   0,
				"users_state_offline":     0,
				"users_state_online":      0,
			},
		},
		"success when response does not contain sessions and users": {
			prepare: prepareConnOKNoSessionsNoUsers,
			expected: map[string]int64{
				"sessions_local":          0,
				"sessions_remote":         0,
				"sessions_state_active":   0,
				"sessions_state_closing":  0,
				"sessions_state_online":   0,
				"sessions_type_console":   0,
				"sessions_type_graphical": 0,
				"sessions_type_other":     0,
				"users_state_active":      0,
				"users_state_closing":     0,
				"users_state_lingering":   0,
				"users_state_offline":     0,
				"users_state_online":      0,
			},
		},
		"fail when error on list sessions": {
			prepare:  prepareConnErrOnListSessions,
			expected: map[string]int64(nil),
		},
		"fail when error on get session properties": {
			prepare:  prepareConnErrOnGetSessionProperties,
			expected: map[string]int64(nil),
		},
		"fail when error on list users": {
			prepare:  prepareConnErrOnListUsers,
			expected: map[string]int64(nil),
		},
		"fail when error on get user property": {
			prepare:  prepareConnErrOnGetUserProperty,
			expected: map[string]int64(nil),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			require.NoError(t, collr.Init(context.Background()))
			collr.conn = test.prepare()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.expected, mx)
		})
	}
}

func prepareConnOK() *mockConn {
	return &mockConn{
		sessions: []login1.Session{
			{Path: "/org/freedesktop/login1/session/_3156", User: "user1", ID: "123"},
			{Path: "/org/freedesktop/login1/session/_3157", User: "user2", ID: "124"},
			{Path: "/org/freedesktop/login1/session/_3158", User: "user3", ID: "125"},
		},
		users: []login1.User{
			{Path: "/org/freedesktop/login1/user/_1000", Name: "user1", UID: 123},
			{Path: "/org/freedesktop/login1/user/_1001", Name: "user2", UID: 124},
			{Path: "/org/freedesktop/login1/user/_1002", Name: "user3", UID: 125},
		},
		errOnListSessions:         false,
		errOnGetSessionProperties: false,
		errOnListUsers:            false,
		errOnGetUserProperty:      false,
		closeCalled:               false,
	}
}

func prepareConnOKNoSessionsNoUsers() *mockConn {
	conn := prepareConnOK()
	conn.sessions = nil
	conn.users = nil
	return conn
}

func prepareConnErrOnListSessions() *mockConn {
	conn := prepareConnOK()
	conn.errOnListSessions = true
	return conn
}

func prepareConnErrOnGetSessionProperties() *mockConn {
	conn := prepareConnOK()
	conn.errOnGetSessionProperties = true
	return conn
}

func prepareConnErrOnListUsers() *mockConn {
	conn := prepareConnOK()
	conn.errOnListUsers = true
	return conn
}

func prepareConnErrOnGetUserProperty() *mockConn {
	conn := prepareConnOK()
	conn.errOnGetUserProperty = true
	return conn
}

type mockConn struct {
	sessions []login1.Session
	users    []login1.User

	errOnListSessions         bool
	errOnGetSessionProperties bool
	errOnListUsers            bool
	errOnGetUserProperty      bool
	closeCalled               bool
}

func (m *mockConn) Close() {
	m.closeCalled = true
}

func (m *mockConn) ListSessions() ([]login1.Session, error) {
	if m.errOnListSessions {
		return nil, errors.New("mock.ListSessions() error")
	}
	return m.sessions, nil
}

func (m *mockConn) GetSessionProperties(path dbus.ObjectPath) (map[string]dbus.Variant, error) {
	if m.errOnGetSessionProperties {
		return nil, errors.New("mock.GetSessionProperties() error")
	}

	var found bool
	for _, s := range m.sessions {
		if s.Path == path {
			found = true
			break
		}
	}

	if !found {
		return nil, errors.New("mock.GetUserProperty(): session is not found")
	}

	return map[string]dbus.Variant{
		"Remote": dbus.MakeVariant("true"),
		"Type":   dbus.MakeVariant("tty"),
		"State":  dbus.MakeVariant("online"),
	}, nil
}

func (m *mockConn) ListUsers() ([]login1.User, error) {
	if m.errOnListUsers {
		return nil, errors.New("mock.ListUsers() error")
	}
	return m.users, nil
}

func (m *mockConn) GetUserProperty(path dbus.ObjectPath, _ string) (*dbus.Variant, error) {
	if m.errOnGetUserProperty {
		return nil, errors.New("mock.GetUserProperty() error")
	}

	var found bool
	for _, u := range m.users {
		if u.Path == path {
			found = true
			break
		}
	}

	if !found {
		return nil, errors.New("mock.GetUserProperty(): user is not found")
	}

	v := dbus.MakeVariant("active")
	return &v, nil
}
