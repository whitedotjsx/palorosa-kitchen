//go:build windows

package main

import (
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"golang.org/x/sys/windows"
)

// restartWaitFlag marks the copy started by relaunch: it waits for the
// previous process to exit before taking the single-instance mutex.
const restartWaitFlag = "--wait-pid"

// createNoWindow keeps the relaunched copy from flashing a console.
const createNoWindow = 0x08000000

// parseRestartWait returns the process id passed with restartWaitFlag, or 0.
func parseRestartWait(args []string) uint32 {
	for index := 0; index < len(args)-1; index++ {
		if args[index] != restartWaitFlag {
			continue
		}
		if pid, err := strconv.ParseUint(args[index+1], 10, 32); err == nil {
			return uint32(pid)
		}
	}
	return 0
}

// waitForProcessExit blocks until the process exits, with a safety timeout so
// a stuck predecessor cannot leave the app dead.
func waitForProcessExit(pid uint32) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	_, _ = windows.WaitForSingleObject(handle, 15000)
}

// relaunch starts a detached copy of the app that waits for this process to
// exit and then runs normally. It backs Ajustes → Guardar y reiniciar.
func relaunch() error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := exec.Command(executable, restartWaitFlag, strconv.Itoa(os.Getpid()))
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return command.Start()
}
