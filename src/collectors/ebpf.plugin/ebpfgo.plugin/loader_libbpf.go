//go:build netdata_ebpf_libbpf

package main

import "github.com/netdata/netdata/src/collectors/ebpf.plugin/ebpfgo.plugin/libbpfloader"

func openLibbpfObject(path string) (*libbpfloader.Object, error) {
	return libbpfloader.OpenObject(path)
}

func loadLibbpfObject(obj *libbpfloader.Object) error {
	return obj.Load()
}

func closeLibbpfObject(obj *libbpfloader.Object) {
	if obj != nil {
		obj.Close()
	}
}

func parseLibbpfBTF(filename string) (*libbpfloader.BTF, error) {
	return libbpfloader.ParseBTFFile(filename)
}

func loadLibbpfBTF(path, filename string) (*libbpfloader.BTF, error) {
	return libbpfloader.LoadBTFFile(path, filename)
}

func freeLibbpfBTF(btf *libbpfloader.BTF) {
	if btf != nil {
		btf.Free()
	}
}

func isFunctionInsideLibbpfBTF(btf *libbpfloader.BTF, function string) (bool, error) {
	return libbpfloader.IsFunctionInsideBTF(btf, function)
}
