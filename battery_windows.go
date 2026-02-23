//go:build windows

package main

import (
	"fmt"
	"time"
	"unsafe"
)

// ReadBatteryINCA reads battery percent + charging flag on the *control path* (mi_02).
// It does SetFeature(0x83) once then polls GetFeature on the same handle,
// skipping ACK frames (a0) until a data frame contains: a1 00 02 02 00 83 [chg] [pct].
func ReadBatteryINCA(dev LamzuDeviceInfo) (percent int, charging bool, ok bool) {
	if dev.Path == "" || dev.FeatureLen == 0 {
		return 0, false, false
	}
	flen := int(dev.FeatureLen)

	h, err := openHIDPath(dev.Path)
	if err != nil {
		return 0, false, false
	}
	defer closeHandle(h)

	// --- SetFeature: 000002020083... (ReportID=0) ---
	set := make([]byte, flen)
	set[0] = 0x00
	set[1] = 0x00
	set[2] = 0x00
	set[3] = 0x02
	set[4] = 0x02
	set[5] = 0x00
	set[6] = 0x83

	r1, _, _ := procHidDSetFeature_HID.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&set[0])),
		uintptr(len(set)),
	)
	if r1 == 0 {
		return 0, false, false
	}

	// --- Poll GetFeature on the same handle ---
	// Empirically needed because device may reply ACK (a0) / current-state pages first.
	for attempt := 0; attempt < 25; attempt++ {
		buf := make([]byte, flen)
		buf[0] = 0x00 // reportID=0

		r2, _, _ := procHidDGetFeature_HID.Call(
			uintptr(h),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(len(buf)),
		)
		if r2 == 0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// For reportID=0, Windows typically returns buf[0]=0; data starts at buf[1].
		data := buf
		if len(buf) >= 2 && buf[0] == 0x00 {
			data = buf[1:]
		}

		// Skip ACK / pending frames (a0)
		if len(data) > 0 && data[0] == 0xA0 {
			time.Sleep(10 * time.Millisecond)
			continue
		}

		// Find battery block: a1 00 02 02 00 83 [chg] [pct]
		for i := 0; i+8 <= len(data); i++ {
			if data[i] == 0xA1 && data[i+1] == 0x00 &&
				data[i+2] == 0x02 && data[i+3] == 0x02 &&
				data[i+4] == 0x00 && data[i+5] == 0x83 {
				chg := data[i+6]
				pct := data[i+7]
				// sanity
				if (chg == 0 || chg == 1) && pct <= 100 {
					return int(pct), chg == 1, true
				}
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	return 0, false, false
}

func PrintBatteryINCA(dev LamzuDeviceInfo) {
	pct, chg, ok := ReadBatteryINCA(dev)
	if !ok {
		fmt.Printf("🔋 INCA Battery: N/A\n")
		return
	}
	state := "discharging"
	if chg {
		state = "charging"
	}
	fmt.Printf("🔋 INCA Battery: %d%% (%s)\n", pct, state)
}
