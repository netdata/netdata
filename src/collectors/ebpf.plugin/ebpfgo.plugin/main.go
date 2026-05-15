package main

import (
	"fmt"
	"os"
	"strconv"
)

func main() {
	updateEvery := 0
	if len(os.Args) > 1 {
		if parsed, err := strconv.Atoi(os.Args[1]); err == nil && parsed > 0 {
			updateEvery = parsed
		}
	}

	handle, err := LoadCachestatLegacyFromSystem()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat load failed: %v\n", err)
		os.Exit(1)
	}

	if updateEvery <= 0 {
		updateEvery = handle.UpdateEvery
	}
	if updateEvery <= 0 {
		updateEvery = cachestatDefaultUpdateEvery
	}
	handle.UpdateEvery = updateEvery

	runCachestatPlugin(handle, updateEvery)
}
