//go:build !netdata_ebpf_libbpf

package libbpfloader

type Object struct {
	Path string
}

type BTF struct {
	Path string
}

type CachestatRuntime struct{}

type SocketRuntime struct{}

func OpenObject(path string) (*Object, error) {
	return nil, ErrDisabled
}

func (o *Object) Load() error {
	return ErrDisabled
}

func (o *Object) Close() {
	// No-op in the disabled build because no libbpf object was opened.
}

func ParseBTFFile(filename string) (*BTF, error) {
	return nil, ErrDisabled
}

func LoadBTFFile(path, filename string) (*BTF, error) {
	return nil, ErrDisabled
}

func (b *BTF) Free() {
	// No-op in the disabled build because no BTF file was loaded.
}

func IsFunctionInsideBTF(file *BTF, function string) (bool, error) {
	return false, ErrDisabled
}

func NewCachestatRuntime(path string, useCore bool) (*CachestatRuntime, error) {
	_ = path
	_ = useCore
	return nil, ErrDisabled
}

func (r *CachestatRuntime) Prepare(pidTableSize uint32, mapsPerCore bool, accountFunction string) error {
	_ = pidTableSize
	_ = mapsPerCore
	return ErrDisabled
}

func (r *CachestatRuntime) Load() error {
	return ErrDisabled
}

func (r *CachestatRuntime) Attach(accountFunction string) error {
	_ = accountFunction
	return ErrDisabled
}

func (r *CachestatRuntime) UpdateController(appsEnabled bool, appsLevel int) error {
	_ = appsEnabled
	_ = appsLevel
	return ErrDisabled
}

func (r *CachestatRuntime) Snapshot(mapsPerCore bool) (CachestatSnapshot, error) {
	_ = mapsPerCore
	return CachestatSnapshot{}, ErrDisabled
}

func (r *CachestatRuntime) SnapshotApps(mapsPerCore bool) ([]CachestatAppSnapshot, error) {
	_ = mapsPerCore
	return nil, ErrDisabled
}

func (r *CachestatRuntime) DeletePid(pid uint32) error {
	_ = pid
	return ErrDisabled
}

func (r *CachestatRuntime) DeletePids(pids []uint32) error {
	_ = pids
	return ErrDisabled
}

func (r *CachestatRuntime) Close() {
	// No-op in the disabled build because the runtime never acquired native resources.
}

// PidIsAlive is the package-level liveness probe; on the disabled build the
// runtime never opens a BPF map and there is no PIDs to evict, so a
// conservative "alive" answer keeps the existing behaviour (no eviction).
func PidIsAlive(pid uint32) bool {
	_ = pid
	return true
}

func NewSocketRuntime(path string, useCore bool) (*SocketRuntime, error) {
	_ = path
	_ = useCore
	return nil, ErrDisabled
}

func (r *SocketRuntime) Prepare(mapsPerCore bool) error {
	_ = mapsPerCore
	return ErrDisabled
}

func (r *SocketRuntime) Load() error {
	return ErrDisabled
}

func (r *SocketRuntime) Attach() error {
	return ErrDisabled
}

func (r *SocketRuntime) Snapshot(mapsPerCore bool) (SocketSnapshot, error) {
	_ = mapsPerCore
	return SocketSnapshot{}, ErrDisabled
}

func (r *SocketRuntime) SnapshotPerPID() ([]SocketPIDEntry, error) {
	return nil, ErrDisabled
}

func (r *SocketRuntime) Close() {
	// No-op in the disabled build because the runtime never acquired native resources.
}
