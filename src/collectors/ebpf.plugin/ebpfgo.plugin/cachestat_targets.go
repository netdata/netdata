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

func resolveCachestatTargets() (CachestatTargets, error) {
	targets := defaultCachestatTargets()
	if err := targets.ResolveAccountPageTarget(); err != nil {
		return CachestatTargets{}, err
	}

	return targets, nil
}

func (t *CachestatTargets) ResolveAccountPageTarget() error {
	name, err := selectCachestatAccountPageTarget(t.AccountPage)
	if err != nil {
		return err
	}

	t.AccountPageDirtied.Name = name
	return nil
}

// selectCachestatAccountPageTarget resolves the account-page target from the
// live symbol table.  Unlike dcstat, an unresolvable target IS fatal here: the
// caller has no usable default (AccountPageDirtied ships with an empty name).
func selectCachestatAccountPageTarget(candidates []string) (string, error) {
	symbols, err := openProcKallsyms()
	if err != nil {
		return "", err
	}
	defer symbols.Close()

	return selectCachestatAccountPageTargetFromReader(candidates, symbols)
}

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
