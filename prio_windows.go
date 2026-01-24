//go:build windows

package main

import (
	"log"
	"syscall"
	"unsafe"
)

var (
	kernel32DLL = syscall.NewLazyDLL("kernel32.dll")

	procGetCurrentProcess     = kernel32DLL.NewProc("GetCurrentProcess")
	procGetCurrentThread      = kernel32DLL.NewProc("GetCurrentThread")
	procSetPriorityClass      = kernel32DLL.NewProc("SetPriorityClass")
	procSetThreadPriority     = kernel32DLL.NewProc("SetThreadPriority")
	procSetProcessInformation = kernel32DLL.NewProc("SetProcessInformation")
	procSetThreadInformation  = kernel32DLL.NewProc("SetThreadInformation")
)

const (
	BELOW_NORMAL_PRIORITY_CLASS   = 0x00004000
	PROCESS_MODE_BACKGROUND_BEGIN = 0x00100000

	THREAD_PRIORITY_LOWEST       int32 = -2
	THREAD_MODE_BACKGROUND_BEGIN int32 = 0x00010000

	ProcessPowerThrottling = 4
	ThreadPowerThrottling  = 5

	PROCESS_POWER_THROTTLING_CURRENT_VERSION = 1
	PROCESS_POWER_THROTTLING_EXECUTION_SPEED = 0x1
	THREAD_POWER_THROTTLING_CURRENT_VERSION  = 1
	THREAD_POWER_THROTTLING_EXECUTION_SPEED  = 0x1
)

type PROCESS_POWER_THROTTLING_STATE struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}
type THREAD_POWER_THROTTLING_STATE struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

func u32ptrFromI32(v int32) uintptr { return uintptr(uint32(v)) }

func setLowPriorityDefaults(enableBackgroundMode bool, enableEcoQoS bool) {
	hProc, _, _ := procGetCurrentProcess.Call()
	hThread, _, _ := procGetCurrentThread.Call()

	if r, _, e := procSetPriorityClass.Call(hProc, uintptr(BELOW_NORMAL_PRIORITY_CLASS)); r == 0 {
		log.Printf("[PRIO] SetPriorityClass(BELOW_NORMAL) failed: %v", e)
	}
	if r, _, e := procSetThreadPriority.Call(hThread, u32ptrFromI32(THREAD_PRIORITY_LOWEST)); r == 0 {
		log.Printf("[PRIO] SetThreadPriority(LOWEST) failed: %v", e)
	}

	if enableBackgroundMode {
		_, _, _ = procSetPriorityClass.Call(hProc, uintptr(PROCESS_MODE_BACKGROUND_BEGIN))
		_, _, _ = procSetThreadPriority.Call(hThread, u32ptrFromI32(THREAD_MODE_BACKGROUND_BEGIN))
	}

	if enableEcoQoS {
		setProcessPowerThrottling(hProc)
		setThreadPowerThrottling(hThread)
	}
}

func setProcessPowerThrottling(hProc uintptr) {
	state := PROCESS_POWER_THROTTLING_STATE{
		Version:     PROCESS_POWER_THROTTLING_CURRENT_VERSION,
		ControlMask: PROCESS_POWER_THROTTLING_EXECUTION_SPEED,
		StateMask:   PROCESS_POWER_THROTTLING_EXECUTION_SPEED,
	}
	_, _, _ = procSetProcessInformation.Call(
		hProc,
		uintptr(ProcessPowerThrottling),
		uintptr(unsafe.Pointer(&state)),
		unsafe.Sizeof(state),
	)
}

func setThreadPowerThrottling(hThread uintptr) {
	state := THREAD_POWER_THROTTLING_STATE{
		Version:     THREAD_POWER_THROTTLING_CURRENT_VERSION,
		ControlMask: THREAD_POWER_THROTTLING_EXECUTION_SPEED,
		StateMask:   THREAD_POWER_THROTTLING_EXECUTION_SPEED,
	}
	_, _, _ = procSetThreadInformation.Call(
		hThread,
		uintptr(ThreadPowerThrottling),
		uintptr(unsafe.Pointer(&state)),
		unsafe.Sizeof(state),
	)
}
