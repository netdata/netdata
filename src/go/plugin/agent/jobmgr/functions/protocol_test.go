// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"bytes"
	"errors"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestFunctionPublicationFrame(t *testing.T) {
	var output bytes.Buffer
	owner, err := lifecycle.NewFrameOwner(&output)
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewFramePublicationPort(7, owner)
	if err != nil {
		t.Fatal(err)
	}
	record := PublicationRecord{
		Name: "module:method", Generation: 3, Timeout: 60,
		Help: "method help", Tags: "top", Access: "0x0013",
		Priority: 100, Version: 3,
	}
	handle, err := port.Publish(record)
	if err != nil {
		t.Fatal(err)
	}
	if handle != (PublicationHandle{
		ID: 1, Epoch: 7, Generation: 3, Name: "module:method",
	}) {
		t.Fatalf("handle=%+v", handle)
	}
	if err := port.Withdraw(handle); err != nil {
		t.Fatal(err)
	}
	want := "" +
		"FUNCTION GLOBAL \"module:method\" 60 \"method help\" \"top\" 0x0013 100 3\n\n" +
		"FUNCTION_DEL GLOBAL \"module:method\"\n\n"
	if output.String() != want {
		t.Fatalf("output=%q want=%q", output.String(), want)
	}
	if err := port.Withdraw(handle); err == nil {
		t.Fatal("duplicate withdrawal was accepted")
	}
}

func TestFunctionPublicationFrameRejectsInjection(t *testing.T) {
	tests := map[string]struct {
		mutate func(*PublicationRecord)
	}{
		"quote in name": {
			mutate: func(record *PublicationRecord) { record.Name = `module:"method` },
		},
		"space in name": {
			mutate: func(record *PublicationRecord) { record.Name = "module: method" },
		},
		"backslash in name": {
			mutate: func(record *PublicationRecord) { record.Name = `module:\method` },
		},
		"newline in help": {
			mutate: func(record *PublicationRecord) { record.Help = "help\nFUNCTION_DEL" },
		},
		"nul in tags": {
			mutate: func(record *PublicationRecord) { record.Tags = "top\x00hidden" },
		},
		"uppercase access": {
			mutate: func(record *PublicationRecord) { record.Access = "0x00AF" },
		},
		"wide access": {
			mutate: func(record *PublicationRecord) { record.Access = "0x00000" },
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			owner, err := lifecycle.NewFrameOwner(&output)
			if err != nil {
				t.Fatal(err)
			}
			port, err := NewFramePublicationPort(1, owner)
			if err != nil {
				t.Fatal(err)
			}
			record := PublicationRecord{
				Name: "module:method", Generation: 1, Timeout: 1,
				Help: "help", Tags: "top", Access: "0x0000",
			}
			test.mutate(&record)
			if _, err := port.Publish(record); err == nil {
				t.Fatal("invalid registration was accepted")
			}
			if output.Len() != 0 {
				t.Fatalf("invalid registration wrote %q", output.String())
			}
		})
	}
}

func TestFunctionPublicationFrameNoHandleBeforeCommit(t *testing.T) {
	writer := &failingFunctionFrameWriter{err: errors.New("write failed")}
	owner, err := lifecycle.NewFrameOwner(writer)
	if err != nil {
		t.Fatal(err)
	}
	port, err := NewFramePublicationPort(1, owner)
	if err != nil {
		t.Fatal(err)
	}
	record := PublicationRecord{
		Name: "module:method", Generation: 1, Timeout: 1,
		Help: "help", Tags: "top", Access: "0x0000",
	}
	handle, err := port.Publish(record)
	if err == nil || handle != (PublicationHandle{}) {
		t.Fatalf("handle=%+v err=%v", handle, err)
	}
	if census := owner.Census(); !census.Poisoned || census.Commits != 0 {
		t.Fatalf("frame census=%+v", census)
	}
}

type failingFunctionFrameWriter struct {
	err error
}

func (writer *failingFunctionFrameWriter) Write(payload []byte) (int, error) {
	return 0, writer.err
}
