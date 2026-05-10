//go:build netdata_ebpf_libbpf

package main

func LoadCachestatLegacy(cfg CachestatLegacyConfig) (*CachestatLegacyHandle, error) {
	plan := BuildCachestatLegacyPlan(cfg)

	obj, err := openLibbpfObject(plan.ObjectPath)
	if err != nil {
		return nil, err
	}

	if err := loadLibbpfObject(obj); err != nil {
		closeLibbpfObject(obj)
		return nil, err
	}

	return &CachestatLegacyHandle{
		Plan:   plan,
		Object: obj,
	}, nil
}

func LoadCachestatLegacyFromSystem() (*CachestatLegacyHandle, error) {
	cfg, err := resolveCachestatLegacyConfig()
	if err != nil {
		return nil, err
	}

	return LoadCachestatLegacy(cfg)
}
