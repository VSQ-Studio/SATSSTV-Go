//go:build windows

// Package serialptt controls RTS or DTR on a dedicated Windows COM port.
package serialptt

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	genericRead   = 0x80000000
	genericWrite  = 0x40000000
	openExisting  = 3
	setRTS        = 3
	clearRTS      = 4
	setDTR        = 5
	clearDTR      = 6
	invalidHandle = ^uintptr(0)
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procCreateFileW    = kernel32.NewProc("CreateFileW")
	procEscapeCommFunc = kernel32.NewProc("EscapeCommFunction")
	procCloseHandle    = kernel32.NewProc("CloseHandle")
)

type Controller struct {
	mu         sync.Mutex
	handle     syscall.Handle
	port       string
	mode       string
	activeHigh bool
}

func New() *Controller { return &Controller{} }

// Set opens and keeps the COM handle for the duration of the keyed interval.
// mode is rts or dtr; activeHigh determines the electrical TX level.
func (c *Controller) Set(port, mode string, enabled, activeHigh bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	port = normalizePort(port)
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "rts" && mode != "dtr" {
		return fmt.Errorf("unsupported serial PTT signal %q", mode)
	}
	if enabled && port == "" {
		return errors.New("serial PTT requires a COM port such as COM3")
	}
	if enabled {
		if c.handle != 0 && (!strings.EqualFold(c.port, port) || c.mode != mode || c.activeHigh != activeHigh) {
			if err := c.releaseLocked(); err != nil {
				return err
			}
		}
		if c.handle == 0 {
			h, err := openPort(port)
			if err != nil {
				return err
			}
			c.handle, c.port, c.mode, c.activeHigh = h, port, mode, activeHigh
			if err := escape(h, clearRTS); err != nil {
				_ = c.releaseLocked()
				return err
			}
			if err := escape(h, clearDTR); err != nil {
				_ = c.releaseLocked()
				return err
			}
		}
		if err := setSignal(c.handle, c.mode, true, c.activeHigh); err != nil {
			_ = c.releaseLocked()
			return err
		}
		return nil
	}
	return c.releaseLocked()
}

func (c *Controller) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.releaseLocked()
}

func (c *Controller) releaseLocked() error {
	if c.handle == 0 {
		return nil
	}
	lineErr := setSignal(c.handle, c.mode, false, c.activeHigh)
	r1, _, closeErr := procCloseHandle.Call(uintptr(c.handle))
	c.handle, c.port, c.mode, c.activeHigh = 0, "", "", false
	if r1 == 0 {
		if closeErr == syscall.Errno(0) {
			closeErr = errors.New("CloseHandle failed")
		}
		closeErr = fmt.Errorf("close COM port: %w", closeErr)
	}
	if lineErr != nil {
		return lineErr
	}
	if r1 == 0 {
		return closeErr
	}
	return nil
}

func normalizePort(port string) string {
	port = strings.TrimSpace(port)
	if port != "" && !strings.HasPrefix(strings.ToLower(port), `\\.\`) {
		port = `\\.\` + port
	}
	return port
}

func openPort(port string) (syscall.Handle, error) {
	ptr, err := syscall.UTF16PtrFromString(port)
	if err != nil {
		return 0, fmt.Errorf("invalid COM port: %w", err)
	}
	r1, _, callErr := procCreateFileW.Call(uintptr(unsafe.Pointer(ptr)), genericRead|genericWrite, 0, 0, openExisting, 0, 0)
	if r1 == invalidHandle {
		if callErr == syscall.Errno(0) {
			callErr = errors.New("CreateFileW returned an invalid handle")
		}
		return 0, fmt.Errorf("open %s: %w", port, callErr)
	}
	return syscall.Handle(r1), nil
}

func setSignal(handle syscall.Handle, mode string, enabled, activeHigh bool) error {
	high := enabled == activeHigh
	command := clearRTS
	if mode == "rts" {
		if high {
			command = setRTS
		}
	} else if mode == "dtr" {
		command = clearDTR
		if high {
			command = setDTR
		}
	} else {
		return fmt.Errorf("unsupported PTT signal %q", mode)
	}
	return escape(handle, uintptr(command))
}

func escape(handle syscall.Handle, command uintptr) error {
	r1, _, callErr := procEscapeCommFunc.Call(uintptr(handle), command)
	if r1 == 0 {
		if callErr == syscall.Errno(0) {
			callErr = errors.New("EscapeCommFunction failed")
		}
		return fmt.Errorf("set RTS/DTR signal: %w", callErr)
	}
	return nil
}
