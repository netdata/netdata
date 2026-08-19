package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
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

func selectCachestatAccountPageTarget(candidates []string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no cachestat account_page candidates configured")
	}

	file, err := os.Open("/proc/kallsyms")
	if err != nil {
		return "", err
	}
	defer file.Close()

	return selectCachestatAccountPageTargetFromFile(candidates, file)
}

func selectCachestatAccountPageTargetFromFile(candidates []string, file *os.File) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("no cachestat account_page candidates configured")
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 {
			continue
		}

		if !isProbeableKallsymsType(fields[1]) {
			continue
		}

		symbol := fields[2]
		for _, candidate := range candidates {
			if candidate == symbol {
				return candidate, nil
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", fmt.Errorf("no cachestat account_page target found")
}

func isProbeableKallsymsType(value string) bool {
	switch value {
	case "T", "t", "W", "w":
		return true
	default:
		return false
	}
}
