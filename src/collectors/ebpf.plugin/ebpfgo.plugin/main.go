package main

import (
	"fmt"
	"os"
)

func main() {
	handle, err := LoadCachestatLegacyFromSystem()
	if err != nil {
		fmt.Fprintf(os.Stderr, "ebpf-go.plugin: cachestat legacy load failed: %v\n", err)
	}

	runCachestatPlugin(handle)
}
