package main

type ProcessTarget struct {
	Name string
	Mode RunMode
}

type ProcessTargets struct {
	ReleaseTask ProcessTarget
	SysClone    ProcessTarget
	SysClone3   ProcessTarget
	SysFork     ProcessTarget
	KernelClone ProcessTarget
}

func defaultProcessTargets() ProcessTargets {
	return ProcessTargets{
		ReleaseTask: ProcessTarget{Name: "release_task", Mode: RunModeEntry},
		SysClone:    ProcessTarget{Name: "__x64_sys_clone", Mode: RunModeEntry},
		SysClone3:   ProcessTarget{Name: "__x64_sys_clone3", Mode: RunModeEntry},
		SysFork:     ProcessTarget{Name: "_do_fork", Mode: RunModeEntry},
		KernelClone: ProcessTarget{Name: "__do_sys_clone", Mode: RunModeEntry},
	}
}

func resolveProcessTargets() (ProcessTargets, error) {
	targets := defaultProcessTargets()

	// For older kernels (< 5.9.16), use _do_fork; for newer, use __do_sys_clone
	// This will be resolved during libbpf loading based on kernel version

	return targets, nil
}
