// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import "testing"

func TestOperationRequiredDeadlineStartIsNonterminal(t *testing.T) {
	operation, err := NewOperation(1, "uid", SourceFunction, "lane", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.RequireDeadlineStart(); err != nil {
		t.Fatal(err)
	}
	if operation.Child != ChildDeadlineStartPending || operation.CanDisposeTerminal() {
		t.Fatalf("required deadline start state=%d disposable=%v", operation.Child, operation.CanDisposeTerminal())
	}
	ref := TaskRef{Slot: 0, Generation: 1}
	if err := operation.StartChild(ref); err != nil {
		t.Fatal(err)
	}
	if operation.Child != ChildExecuting || operation.Task != ref {
		t.Fatalf("required deadline child start differs: %#v", operation)
	}
}

func TestOperationAbandonedDeadlineStartBecomesTerminal(t *testing.T) {
	operation, err := NewOperation(1, "uid", SourceFunction, "lane", true)
	if err != nil {
		t.Fatal(err)
	}
	if err := operation.RequireDeadlineStart(); err != nil {
		t.Fatal(err)
	}
	if err := operation.AbandonDeadlineStart(); err != nil {
		t.Fatal(err)
	}
	if operation.Child != ChildAbandonedBeforeStart {
		t.Fatalf("abandoned deadline start state=%d", operation.Child)
	}
	if err := operation.CommitResponse(); err != nil {
		t.Fatal(err)
	}
	if !operation.CanDisposeTerminal() {
		t.Fatal("abandoned deadline start did not become terminal")
	}
}
