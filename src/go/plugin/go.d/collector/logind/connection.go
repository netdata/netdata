// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logind

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/login1"
	"github.com/godbus/dbus/v5"
)

type logindConnection interface {
	Close()

	ListSessions() ([]login1.Session, error)
	GetSessionProperties(dbus.ObjectPath) (map[string]dbus.Variant, error)

	ListUsers() ([]login1.User, error)
	GetUserProperty(dbus.ObjectPath, string) (*dbus.Variant, error)
}

func newLogindConnection(timeout time.Duration) (logindConnection, error) {
	conn, err := login1.New()
	if err != nil {
		return nil, err
	}
	return &logindDBusConnection{
		conn:    conn,
		timeout: timeout,
	}, nil
}

type logindDBusConnection struct {
	conn    *login1.Conn
	timeout time.Duration
}

func (c *logindDBusConnection) Close() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}

func (c *logindDBusConnection) ListSessions() ([]login1.Session, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.conn.ListSessionsContext(ctx)
}

func (c *logindDBusConnection) ListUsers() ([]login1.User, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.conn.ListUsersContext(ctx)
}

func (c *logindDBusConnection) GetSessionProperties(path dbus.ObjectPath) (map[string]dbus.Variant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.conn.GetSessionPropertiesContext(ctx, path)
}

func (c *logindDBusConnection) GetUserProperty(path dbus.ObjectPath, property string) (*dbus.Variant, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	return c.conn.GetUserPropertyContext(ctx, path, property)
}
