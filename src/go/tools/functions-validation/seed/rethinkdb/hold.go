// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/rethinkdb/rethinkdb-go.v6"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:28015", "RethinkDB address")
	duration := flag.Duration("duration", 30*time.Second, "How long to keep the changefeed open")
	flag.Parse()

	sess, err := rethinkdb.Connect(rethinkdb.ConnectOpts{Address: *addr})
	if err != nil {
		fatalf("connect: %v", err)
	}
	defer func() { _ = sess.Close() }()

	ensureDB(sess, "netdata")
	ensureTable(sess, "netdata", "demo")
	insertSeed(sess, "netdata", "demo")

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()

	cur, err := rethinkdb.DB("netdata").Table("demo").Changes().Run(sess, rethinkdb.RunOpts{Context: ctx})
	if err != nil {
		fatalf("start changefeed: %v", err)
	}
	defer func() { _ = cur.Close() }()

	<-ctx.Done()
}

func ensureDB(sess *rethinkdb.Session, name string) {
	if _, err := rethinkdb.DBCreate(name).RunWrite(sess); err != nil {
		if !isAlreadyExists(err) {
			fatalf("db create: %v", err)
		}
	}
}

func ensureTable(sess *rethinkdb.Session, db, table string) {
	if _, err := rethinkdb.DB(db).TableCreate(table).RunWrite(sess); err != nil {
		if !isAlreadyExists(err) {
			fatalf("table create: %v", err)
		}
	}
}

func insertSeed(sess *rethinkdb.Session, db, table string) {
	_, _ = rethinkdb.DB(db).Table(table).Insert(map[string]any{
		"id":   "seed",
		"name": "alpha",
	}).RunWrite(sess)
}

func isAlreadyExists(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "already exists") || strings.Contains(err.Error(), "Duplicate"))
}

func fatalf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
