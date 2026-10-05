//go:build windows

// Package singleinstance keeps one copy of the app running: a second launch
// signals the running instance to show its window and exits.
package singleinstance

import (
	"errors"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	mu          sync.Mutex
	mutexHandle windows.Handle
	eventHandle windows.Handle
)

// Acquire takes the named mutex. It reports whether another instance already
// holds it. The handle is kept for the process lifetime.
func Acquire(name string) (bool, error) {
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false, err
	}
	handle, err := windows.CreateMutex(nil, false, pointer)
	if err != nil && !errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		return false, err
	}
	mu.Lock()
	mutexHandle = handle
	mu.Unlock()
	return errors.Is(err, windows.ERROR_ALREADY_EXISTS), nil
}

// SignalShow wakes the running instance so it shows its window.
func SignalShow(name string) error {
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return err
	}
	handle, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, pointer)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.SetEvent(handle)
}

// ListenShow waits for a show signal and calls onShow each time.
func ListenShow(name string, onShow func()) {
	pointer, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return
	}
	handle, err := windows.CreateEvent(nil, 0, 0, pointer)
	if err != nil {
		return
	}
	mu.Lock()
	eventHandle = handle
	mu.Unlock()
	go func() {
		for {
			status, err := windows.WaitForSingleObject(handle, windows.INFINITE)
			if err != nil || status != windows.WAIT_OBJECT_0 {
				continue
			}
			onShow()
		}
	}()
}
