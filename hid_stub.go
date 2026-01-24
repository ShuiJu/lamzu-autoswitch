//go:build !windows

package main

import "errors"

type LamzuDeviceInfo struct {
	Path         string
	VID          uint16
	PID          uint16
	Manufacturer string
	Product      string
}

func EnumerateLamzuDevices() ([]LamzuDeviceInfo, error) {
	return nil, errors.New("HID enumeration is only supported on Windows")
}

func FindOneLamzuDevice() (LamzuDeviceInfo, error) {
	return LamzuDeviceInfo{}, errors.New("HID enumeration is only supported on Windows")
}

func ApplyLamzuSetting(path string, perf PerfMode, poll PollingRate, motionSync bool, sleepSec int) error {
	return errors.New("HID feature report is only supported on Windows")
}

func EnumerateAllHidDevices() ([]LamzuDeviceInfo, error) {
	return nil, errors.New("HID enumeration is only supported on Windows")
}
