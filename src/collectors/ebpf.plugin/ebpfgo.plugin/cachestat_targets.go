package main

import (
	"fmt"
	"io"
)

type CachestatTarget struct {
	Name string
	Mode RunMode
}

type CachestatTargets struct {
	AddToPageCacheLru  CachestatTarget
	MarkPageAccessed   CachestatTarget
	AccountPageDirtied CachestatTarget
	MarkBufferDirty    CachestatTarget
	AccountPage        []string
}

func defaultCachestatTargets() CachestatTargets {
	return CachestatTargets{
		AddToPageCacheLru: CachestatTarget{
			Name: "add_to_page_cache_lru",
			Mode: RunModeEntry,
		},
		MarkPageAccessed: CachestatTarget{
			Name: "mark_page_accessed",
			Mode: RunModeEntry,
		},
		AccountPageDirtied: CachestatTarget{
			Mode: RunModeEntry,
		},
		MarkBufferDirty: CachestatTarget{
			Name: "mark_buffer_dirty",
			Mode: RunModeEntry,
		},
		AccountPage: []string{
			"account_page_dirtied",
			"__set_page_dirty",
			"__folio_mark_dirty",
		},
	}
}

func (t *CachestatTargets) ResolveAllFromReader(symbols io.Reader) error {
	resolved, err := selectKallsymsCandidateSets(
		symbols,
		[]string{"filemap_add_folio", "add_to_page_cache_lru"},
		[]string{"folio_mark_accessed", "mark_page_accessed"},
		t.AccountPage,
	)
	if err != nil {
		return fmt.Errorf("reading kernel symbols: %w", err)
	}
	for i, candidates := range [][]string{
		{"filemap_add_folio", "add_to_page_cache_lru"},
		{"folio_mark_accessed", "mark_page_accessed"},
		t.AccountPage,
	} {
		if resolved[i] == "" {
			return fmt.Errorf("no cachestat target found among %v", candidates)
		}
	}
	t.AddToPageCacheLru.Name = resolved[0]
	t.MarkPageAccessed.Name = resolved[1]
	t.AccountPageDirtied.Name = resolved[2]
	return nil
}

func selectCachestatKernelTargetFromReader(candidates []string, symbols io.Reader) (string, error) {
	name, err := selectKallsymsCandidate(candidates, symbols)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("no cachestat target found among %v", candidates)
	}
	return name, nil
}

func resolveCachestatTargets() (CachestatTargets, error) {
	targets := defaultCachestatTargets()
	symbols, err := openProcKallsyms()
	if err != nil {
		return CachestatTargets{}, err
	}
	defer symbols.Close()
	if err := targets.ResolveAllFromReader(symbols); err != nil {
		return CachestatTargets{}, err
	}

	return targets, nil
}

// Unlike dcstat, an unresolvable account-page target is fatal here: the
// caller has no usable default (AccountPageDirtied ships with an empty name).
func selectCachestatAccountPageTargetFromReader(candidates []string, r io.Reader) (string, error) {
	name, err := selectKallsymsCandidate(candidates, r)
	if err != nil {
		return "", err
	}
	if name == "" {
		return "", fmt.Errorf("no cachestat account_page target found")
	}

	return name, nil
}
