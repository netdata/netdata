//go:build !unix || !cgo

package main

func cgoScaffoldReady() int {
	return 0
}
