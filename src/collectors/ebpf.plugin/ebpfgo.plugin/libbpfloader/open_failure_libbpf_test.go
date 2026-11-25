//go:build netdata_ebpf_libbpf

package libbpfloader

import (
	"os"
	"path/filepath"
	"testing"
)

// A failed open must come back as an error with nothing to close. libbpf 1.x
// returns NULL from a failed bpf_object__open_file(); libbpf 0.x returns an
// error pointer, and closing it crashed the process. useCore is false, so the
// loaders take the object-file path, the only one libbpf 0.x has.
func TestOpenInvalidObjectReturnsError(t *testing.T) {
	dir := t.TempDir()
	notAnObject := filepath.Join(dir, "not-an-object.o")
	if err := os.WriteFile(notAnObject, []byte("not an ELF object\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	objects := map[string]string{
		"missing file":  filepath.Join(dir, "missing.o"),
		"not an object": notAnObject,
	}

	// Each opener returns the close function of whatever it opened.
	openers := map[string]func(path string) (func(), error){
		"cachestat": func(path string) (func(), error) {
			rt, err := NewCachestatRuntime(path, false)
			if err != nil {
				return nil, err
			}
			return rt.Close, nil
		},
		"dcstat": func(path string) (func(), error) {
			rt, err := NewDCStatRuntime(path, false)
			if err != nil {
				return nil, err
			}
			return rt.Close, nil
		},
		"dns": func(path string) (func(), error) {
			rt, err := NewDNSRuntime(path, false, false)
			if err != nil {
				return nil, err
			}
			return rt.Close, nil
		},
		"fd": func(path string) (func(), error) {
			rt, err := NewFDRuntime(path, false, "")
			if err != nil {
				return nil, err
			}
			return rt.Close, nil
		},
		"socket": func(path string) (func(), error) {
			rt, err := NewSocketRuntime(path, false)
			if err != nil {
				return nil, err
			}
			return rt.Close, nil
		},
		"OpenObject": func(path string) (func(), error) {
			obj, err := OpenObject(path)
			if err != nil {
				return nil, err
			}
			return obj.Close, nil
		},
	}

	for name, open := range openers {
		for kind, path := range objects {
			t.Run(name+"/"+kind, func(t *testing.T) {
				closeOpened, err := open(path)
				if err == nil {
					closeOpened()
					t.Fatalf("opening %s succeeded, want an error", path)
				}
			})
		}
	}
}
