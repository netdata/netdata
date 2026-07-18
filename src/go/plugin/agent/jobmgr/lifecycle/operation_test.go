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

func TestOperationResponseCommitPrecedesDisposalAcknowledgement(
	t *testing.T,
) {
	operation, err := NewOperation(
		1,
		"uid",
		SourceFunction,
		"lane",
		true,
	)
	if err != nil {
		t.Fatal(err)
	}
	ref := TaskRef{Slot: 1, Generation: 1}
	if err := operation.StartChild(ref); err != nil {
		t.Fatal(err)
	}
	if err := operation.ResultReady(ref, 1); err != nil {
		t.Fatal(err)
	}
	if err := operation.MarkResponsePending(); err != nil {
		t.Fatal(err)
	}
	if err := operation.ActionPending(ref, 2); err != nil {
		t.Fatal(err)
	}
	if err := operation.ActionAcknowledged(ref, 2); err != nil {
		t.Fatal(err)
	}
	if err := operation.TerminationPending(ref, 3); err != nil {
		t.Fatal(err)
	}
	if err := operation.ChildExited(ref, 3); err != nil {
		t.Fatal(err)
	}
	if operation.CanDisposeTerminal() {
		t.Fatal(
			"disposal acknowledgement made an open response terminal",
		)
	}
	if err := operation.CommitResponse(); err != nil {
		t.Fatal(err)
	}
	if !operation.CanDisposeTerminal() {
		t.Fatal(
			"committed response plus disposal acknowledgement was not terminal",
		)
	}
}
