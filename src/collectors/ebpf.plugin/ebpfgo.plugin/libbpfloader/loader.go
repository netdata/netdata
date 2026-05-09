//go:build !netdata_ebpf_libbpf

package libbpfloader

type Object struct {
	Path string
}

type BTF struct {
	Path string
}

func OpenObject(path string) (*Object, error) {
	return nil, ErrDisabled
}

func (o *Object) Load() error {
	return ErrDisabled
}

func (o *Object) Close() {}

func ParseBTFFile(filename string) (*BTF, error) {
	return nil, ErrDisabled
}

func LoadBTFFile(path, filename string) (*BTF, error) {
	return nil, ErrDisabled
}

func (b *BTF) Free() {}

func IsFunctionInsideBTF(file *BTF, function string) (bool, error) {
	return false, ErrDisabled
}
