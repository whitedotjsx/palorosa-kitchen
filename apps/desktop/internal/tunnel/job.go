//go:build windows

package tunnel

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	jobOnce   sync.Once
	jobHandle windows.Handle
	jobErr    error
)

// lifetimeJob returns a job object that kills every process assigned to it
// when the last handle closes, which happens when this process exits for any
// reason (including being killed by the dev watcher or Task Manager).
func lifetimeJob() (windows.Handle, error) {
	jobOnce.Do(func() {
		handle, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			jobErr = err
			return
		}
		info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
		if _, err := windows.SetInformationJobObject(
			handle,
			windows.JobObjectExtendedLimitInformation,
			uintptr(unsafe.Pointer(&info)),
			uint32(unsafe.Sizeof(info)),
		); err != nil {
			_ = windows.CloseHandle(handle)
			jobErr = err
			return
		}
		jobHandle = handle
	})
	return jobHandle, jobErr
}

// bindToProcessLifetime assigns pid to the lifetime job, so the child dies
// with the app instead of lingering as an orphan tunnel connector.
func bindToProcessLifetime(pid int) error {
	job, err := lifetimeJob()
	if err != nil {
		return err
	}
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
}
