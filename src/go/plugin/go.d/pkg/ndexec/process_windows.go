// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows 10+: assign the job as part of CreateProcess, before any child code
// runs. Start-then-assign permits children to escape the containment boundary.
const procThreadAttributeJobList = 0x0002000d

type windowsProcess struct {
	process *os.Process
	handle  windows.Handle
	job     windows.Handle
}

func startOwnedProcess(path string, args []string, opts ProcessOptions) (processHandle, error) {
	path, err := exec.LookPath(path)
	if err != nil {
		return nil, err
	}
	application, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(append([]string{path}, args...)))
	if err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	owned := false
	defer func() {
		if !owned {
			_ = windows.CloseHandle(job)
		}
	}()
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, err
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	if err := attributes.Update(procThreadAttributeJobList, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return nil, err
	}
	handles := make([]windows.Handle, 3)
	defer func() {
		for _, handle := range handles {
			if handle != 0 {
				_ = windows.CloseHandle(handle)
			}
		}
	}()
	for i, file := range []*os.File{opts.Stdin, opts.Stdout, opts.Stderr} {
		err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(file.Fd()),
			windows.CurrentProcess(), &handles[i], 0, true, windows.DUPLICATE_SAME_ACCESS)
		runtime.KeepAlive(file)
		if err != nil {
			return nil, err
		}
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]),
		uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	startup := windows.StartupInfoEx{}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[2]
	startup.ProcThreadAttributeList = attributes.List()
	var info windows.ProcessInformation
	err = windows.CreateProcess(application, commandLine, nil, nil, true,
		windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT, nil, nil, &startup.StartupInfo, &info)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(job)
	if err != nil {
		return nil, err
	}
	_ = windows.CloseHandle(info.Thread)
	// The creation handle pins identity until FindProcess obtains Go's wait handle.
	process, err := os.FindProcess(int(info.ProcessId))
	if err != nil {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
		owned = true // The rollback has already closed the job.
		_, _ = windows.WaitForSingleObject(info.Process, windows.INFINITE)
		_ = windows.CloseHandle(info.Process)
		return nil, err
	}
	owned = true
	return &windowsProcess{
		process: process,
		handle:  info.Process,
		job:     job,
	}, nil
}

func (p *windowsProcess) observeExit() error {
	event, err := windows.WaitForSingleObject(p.handle, windows.INFINITE)
	if err != nil {
		return err
	}
	if event != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("unexpected process wait result: %d", event)
	}
	return nil
}

func (p *windowsProcess) terminate() error {
	// Process serializes this with release and invokes it exactly once. The job
	// handle, unlike a PID, identifies the owned tree even after its leader exits.
	err := windows.TerminateJobObject(p.job, 1)
	closeErr := windows.CloseHandle(p.job)
	p.job = 0
	return errors.Join(err, closeErr)
}

func (p *windowsProcess) reap() error {
	state, err := p.process.Wait()
	if err != nil {
		return err
	}
	if !state.Success() {
		return &exec.ExitError{
			ProcessState: state,
		}
	}
	return nil
}

func (p *windowsProcess) release() error { return windows.CloseHandle(p.handle) }
