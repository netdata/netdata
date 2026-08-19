//go:build netdata_ebpf_libbpf

package libbpfloader

/*
#include <stdlib.h>

struct bpf_object;
struct btf;

struct bpf_object *netdata_ebpf_open_file(const char *path);
int netdata_ebpf_load_object(struct bpf_object *obj);
void netdata_ebpf_close_object(struct bpf_object *obj);
struct btf *netdata_ebpf_parse_btf_file(const char *filename);
void netdata_ebpf_free_btf(struct btf *file);
int netdata_ebpf_is_function_inside_btf(struct btf *file, const char *function);
*/
import "C"

import (
	"fmt"
	"path/filepath"
	"unsafe"
)

type Object struct {
	ptr *C.struct_bpf_object
}

type BTF struct {
	ptr *C.struct_btf
}

func OpenObject(path string) (*Object, error) {
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))

	obj := C.netdata_ebpf_open_file(cpath)
	if obj == nil {
		return nil, fmt.Errorf("open eBPF object %q failed", path)
	}

	return &Object{ptr: obj}, nil
}

func (o *Object) Load() error {
	if o == nil || o.ptr == nil {
		return ErrDisabled
	}

	if ret := C.netdata_ebpf_load_object(o.ptr); ret != 0 {
		return fmt.Errorf("load eBPF object failed: %d", int(ret))
	}

	return nil
}

func (o *Object) Close() {
	if o == nil || o.ptr == nil {
		return
	}

	C.netdata_ebpf_close_object(o.ptr)
	o.ptr = nil
}

func ParseBTFFile(filename string) (*BTF, error) {
	cfile := C.CString(filename)
	defer C.free(unsafe.Pointer(cfile))

	btf := C.netdata_ebpf_parse_btf_file(cfile)
	if btf == nil {
		return nil, fmt.Errorf("parse BTF file %q failed", filename)
	}

	return &BTF{ptr: btf}, nil
}

func LoadBTFFile(path, filename string) (*BTF, error) {
	return ParseBTFFile(filepath.Join(path, filename))
}

func (b *BTF) Free() {
	if b == nil || b.ptr == nil {
		return
	}

	C.netdata_ebpf_free_btf(b.ptr)
	b.ptr = nil
}

func IsFunctionInsideBTF(file *BTF, function string) (bool, error) {
	if file == nil || file.ptr == nil {
		return false, ErrDisabled
	}

	cfn := C.CString(function)
	defer C.free(unsafe.Pointer(cfn))

	ret := C.netdata_ebpf_is_function_inside_btf(file.ptr, cfn)
	switch ret {
	case 1:
		return true, nil
	case 0:
		return false, nil
	default:
		return false, fmt.Errorf("query BTF for %q failed", function)
	}
}
