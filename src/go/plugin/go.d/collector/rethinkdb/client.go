// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"context"
	"errors"
	"time"

	"gopkg.in/rethinkdb/rethinkdb-go.v6"
)

type rdbConn interface {
	stats() ([][]byte, error)
	close() error
}

func newRethinkdbConn(cfg Config) (rdbConn, error) {
	sess, err := rethinkdb.Connect(rethinkdb.ConnectOpts{
		Address:  cfg.Address,
		Username: cfg.Username,
		Password: cfg.Password,
	})
	if err != nil {
		return nil, err
	}

	client := &rethinkdbClient{
		timeout: cfg.Timeout.Duration(),
		sess:    sess,
	}

	return client, nil
}

type rethinkdbClient struct {
	timeout time.Duration

	sess *rethinkdb.Session
}

func (c *rethinkdbClient) stats() ([][]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	opts := rethinkdb.RunOpts{Context: ctx}

	cur, err := rethinkdb.DB("rethinkdb").Table("stats").Run(c.sess, opts)
	if err != nil {
		return nil, err
	}

	if cur.IsNil() {
		return nil, errors.New("no stats found (cursor is nil)")
	}
	defer func() { _ = cur.Close() }()

	var stats [][]byte
	for {
		bs, ok := cur.NextResponse()
		if !ok {
			break
		}
		stats = append(stats, bs)
	}

	return stats, nil
}

func (c *rethinkdbClient) close() (err error) {
	return c.sess.Close()
}
