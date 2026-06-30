package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSelectCachestatDirtyAccountFunctionFromCandidates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kallsyms")

	if err := os.WriteFile(path, []byte(`
ffffffff81000000 T __set_page_dirty
ffffffff81000001 T account_page_dirtied
`), 0o600); err != nil {
		t.Fatalf("write kallsyms: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open kallsyms: %v", err)
	}
	defer file.Close()

	got, err := selectCachestatAccountPageTargetFromFile([]string{
		"account_page_dirtied",
		"__set_page_dirty",
		"__folio_mark_dirty",
	}, file)
	if err != nil {
		t.Fatalf("resolve account page target: %v", err)
	}

	if got != "__set_page_dirty" {
		t.Fatalf("unexpected target: %q", got)
	}
}

func TestResolveCachestatTargetsUsesDirtyAccountSymbol(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kallsyms")

	if err := os.WriteFile(path, []byte(`
ffffffff81000000 T __folio_mark_dirty
`), 0o600); err != nil {
		t.Fatalf("write kallsyms: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open kallsyms: %v", err)
	}
	defer file.Close()

	got, err := selectCachestatAccountPageTargetFromFile([]string{
		"account_page_dirtied",
		"__set_page_dirty",
		"__folio_mark_dirty",
	}, file)
	if err != nil {
		t.Fatalf("resolve account page target: %v", err)
	}

	if got != "__folio_mark_dirty" {
		t.Fatalf("unexpected target: %q", got)
	}
}

func TestSelectCachestatDirtyAccountFunctionSkipsNonProbeableSymbols(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kallsyms")

	if err := os.WriteFile(path, []byte(`
ffffffff81000000 r __folio_mark_dirty
ffffffff81000001 T __set_page_dirty
`), 0o600); err != nil {
		t.Fatalf("write kallsyms: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open kallsyms: %v", err)
	}
	defer file.Close()

	got, err := selectCachestatAccountPageTargetFromFile([]string{
		"account_page_dirtied",
		"__set_page_dirty",
		"__folio_mark_dirty",
	}, file)
	if err != nil {
		t.Fatalf("resolve account page target: %v", err)
	}

	if got != "__set_page_dirty" {
		t.Fatalf("unexpected target: %q", got)
	}
}

func TestResolveCachestatTargetsFillsAccountPageSlot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kallsyms")

	if err := os.WriteFile(path, []byte(`
ffffffff81000000 T account_page_dirtied
`), 0o600); err != nil {
		t.Fatalf("write kallsyms: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open kallsyms: %v", err)
	}
	defer file.Close()

	targets := defaultCachestatTargets()
	got, err := selectCachestatAccountPageTargetFromFile(targets.AccountPage, file)
	if err != nil {
		t.Fatalf("resolve targets: %v", err)
	}
	targets.AccountPageDirtied.Name = got

	if targets.AccountPageDirtied.Name != "account_page_dirtied" {
		t.Fatalf("unexpected target: %q", targets.AccountPageDirtied.Name)
	}
}
