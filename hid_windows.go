//go:build windows

package main

import (
	"fmt"
	"log"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

type GUID struct {
	Data1 uint32
	Data2 uint16
	Data3 uint16
	Data4 [8]byte
}

type HIDD_ATTRIBUTES struct {
	Size      uint32
	VendorID  uint16
	ProductID uint16
	Version   uint16
}

type SP_DEVICE_INTERFACE_DATA struct {
	CbSize             uint32
	InterfaceClassGuid GUID
	Flags              uint32
	Reserved           uintptr
}

type HIDP_CAPS struct {
	Usage                     uint16
	UsagePage                 uint16
	InputReportByteLength     uint16
	OutputReportByteLength    uint16
	FeatureReportByteLength   uint16
	Reserved                  [17]uint16
	NumberLinkCollectionNodes uint16
	NumberInputButtonCaps     uint16
	NumberInputValueCaps      uint16
	NumberInputDataIndices    uint16
	NumberOutputButtonCaps    uint16
	NumberOutputValueCaps     uint16
	NumberOutputDataIndices   uint16
	NumberFeatureButtonCaps   uint16
	NumberFeatureValueCaps    uint16
	NumberFeatureDataIndices  uint16
}

const ERROR_NO_MORE_ITEMS syscall.Errno = 259
const HIDP_STATUS_SUCCESS uint32 = 0x00110000

var (
	setupapiHID = syscall.NewLazyDLL("setupapi.dll")
	hidDLLHID   = syscall.NewLazyDLL("hid.dll")
	k32HID      = syscall.NewLazyDLL("kernel32.dll")

	procSetupDiGetClassDevsW_HID             = setupapiHID.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces_HID      = setupapiHID.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW_HID = setupapiHID.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList_HID     = setupapiHID.NewProc("SetupDiDestroyDeviceInfoList")

	procHidDGetHidGuid_HID            = hidDLLHID.NewProc("HidD_GetHidGuid")
	procHidDGetAttributes_HID         = hidDLLHID.NewProc("HidD_GetAttributes")
	procHidDGetManufacturerString_HID = hidDLLHID.NewProc("HidD_GetManufacturerString")
	procHidDGetProductString_HID      = hidDLLHID.NewProc("HidD_GetProductString")

	procHidDSetFeature_HID        = hidDLLHID.NewProc("HidD_SetFeature")
	procHidDGetFeature_HID        = hidDLLHID.NewProc("HidD_GetFeature")
	procHidDGetPreparsedData_HID  = hidDLLHID.NewProc("HidD_GetPreparsedData")
	procHidDFreePreparsedData_HID = hidDLLHID.NewProc("HidD_FreePreparsedData")
	procHidPGetCaps_HID           = hidDLLHID.NewProc("HidP_GetCaps")

	procCreateFileW_HID  = k32HID.NewProc("CreateFileW")
	procCloseHandle_HID  = k32HID.NewProc("CloseHandle")
	procGetLastError_HID = k32HID.NewProc("GetLastError")
)

const (
	DIGCF_PRESENT         = 0x00000002
	DIGCF_DEVICEINTERFACE = 0x00000010

	GENERIC_READ  = 0x80000000
	GENERIC_WRITE = 0x40000000

	FILE_SHARE_READ  = 0x00000001
	FILE_SHARE_WRITE = 0x00000002

	OPEN_EXISTING = 3
)

func detailCbSizeW() uint32 {
	if unsafe.Sizeof(uintptr(0)) == 8 {
		return 8
	}
	return 6
}

const detailDevicePathOffset = 4

type LamzuDeviceInfo struct {
	Path         string
	VID          uint16
	PID          uint16
	Manufacturer string
	Product      string
	FeatureLen   uint16
}

func lastErrno() syscall.Errno {
	r1, _, _ := procGetLastError_HID.Call()
	return syscall.Errno(r1)
}

func openHIDPath(path string) (syscall.Handle, error) {
	p16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, _, _ := procCreateFileW_HID.Call(
		uintptr(unsafe.Pointer(p16)),
		uintptr(GENERIC_READ|GENERIC_WRITE),
		uintptr(FILE_SHARE_READ|FILE_SHARE_WRITE),
		0,
		uintptr(OPEN_EXISTING),
		0,
		0,
	)
	if h != 0 && h != uintptr(syscall.InvalidHandle) {
		return syscall.Handle(h), nil
	}
	h2, _, _ := procCreateFileW_HID.Call(
		uintptr(unsafe.Pointer(p16)),
		uintptr(GENERIC_WRITE),
		uintptr(FILE_SHARE_READ|FILE_SHARE_WRITE),
		0,
		uintptr(OPEN_EXISTING),
		0,
		0,
	)
	if h2 != 0 && h2 != uintptr(syscall.InvalidHandle) {
		return syscall.Handle(h2), nil
	}
	return 0, fmt.Errorf("CreateFileW failed: %s (%v)", path, lastErrno())
}

func openHIDPathForQuery(path string) (syscall.Handle, error) {
	p16, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	h, _, _ := procCreateFileW_HID.Call(
		uintptr(unsafe.Pointer(p16)),
		0,
		uintptr(FILE_SHARE_READ|FILE_SHARE_WRITE),
		0,
		uintptr(OPEN_EXISTING),
		0,
		0,
	)
	if h != 0 && h != uintptr(syscall.InvalidHandle) {
		return syscall.Handle(h), nil
	}
	return 0, fmt.Errorf("CreateFileW(query) failed: %s (%v)", path, lastErrno())
}

func closeHandle(h syscall.Handle) {
	procCloseHandle_HID.Call(uintptr(h))
}

func hidGuid() GUID {
	var g GUID
	procHidDGetHidGuid_HID.Call(uintptr(unsafe.Pointer(&g)))
	return g
}

func utf16FromPtr(p *uint16) string {
	if p == nil {
		return ""
	}
	var arr []uint16
	for i := 0; ; i++ {
		u := *(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p)) + uintptr(i*2)))
		if u == 0 {
			break
		}
		arr = append(arr, u)
	}
	return syscall.UTF16ToString(arr)
}

func hidGetString(h syscall.Handle, proc *syscall.LazyProc) string {
	buf := make([]uint16, 256)
	r1, _, _ := proc.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)*2))
	if r1 == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}

func queryCaps(h syscall.Handle) (HIDP_CAPS, error) {
	var pp uintptr
	r1, _, _ := procHidDGetPreparsedData_HID.Call(uintptr(h), uintptr(unsafe.Pointer(&pp)))
	if r1 == 0 || pp == 0 {
		return HIDP_CAPS{}, fmt.Errorf("HidD_GetPreparsedData failed: %v", lastErrno())
	}
	defer procHidDFreePreparsedData_HID.Call(pp)

	var caps HIDP_CAPS
	st, _, _ := procHidPGetCaps_HID.Call(pp, uintptr(unsafe.Pointer(&caps)))
	if uint32(st) != HIDP_STATUS_SUCCESS {
		return HIDP_CAPS{}, fmt.Errorf("HidP_GetCaps failed: 0x%08x", uint32(st))
	}
	return caps, nil
}

func queryDeviceInfo(path string) (LamzuDeviceInfo, bool) {
	h, err := openHIDPathForQuery(path)
	if err != nil {
		return LamzuDeviceInfo{}, false
	}
	defer closeHandle(h)

	var attr HIDD_ATTRIBUTES
	attr.Size = uint32(unsafe.Sizeof(attr))
	r1, _, _ := procHidDGetAttributes_HID.Call(uintptr(h), uintptr(unsafe.Pointer(&attr)))
	if r1 == 0 {
		return LamzuDeviceInfo{}, false
	}

	manu := hidGetString(h, procHidDGetManufacturerString_HID)
	prod := hidGetString(h, procHidDGetProductString_HID)

	caps, _ := queryCaps(h)

	return LamzuDeviceInfo{
		Path: path, VID: attr.VendorID, PID: attr.ProductID,
		Manufacturer: manu, Product: prod,
		FeatureLen: caps.FeatureReportByteLength,
	}, true
}

func sendFeatureReport(path string, report []byte) error {
	h, err := openHIDPath(path)
	if err != nil {
		return err
	}
	defer closeHandle(h)

	r1, _, _ := procHidDSetFeature_HID.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&report[0])),
		uintptr(len(report)),
	)
	if r1 == 0 {
		return fmt.Errorf("HidD_SetFeature failed: %v", lastErrno())
	}
	return nil
}

func getFeature(path string, reportID byte, length int) ([]byte, error) {
	h, err := openHIDPath(path)
	if err != nil {
		return nil, err
	}
	defer closeHandle(h)

	buf := make([]byte, length)
	buf[0] = reportID
	r1, _, _ := procHidDGetFeature_HID.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
	)
	if r1 == 0 {
		return nil, fmt.Errorf("HidD_GetFeature failed: %v", lastErrno())
	}
	return buf, nil
}

func EnumerateLamzuDevices() ([]LamzuDeviceInfo, error) {
	g := hidGuid()

	hDevInfo, _, _ := procSetupDiGetClassDevsW_HID.Call(
		uintptr(unsafe.Pointer(&g)), 0, 0,
		uintptr(DIGCF_PRESENT|DIGCF_DEVICEINTERFACE),
	)
	if hDevInfo == 0 || hDevInfo == uintptr(syscall.InvalidHandle) {
		return nil, fmt.Errorf("SetupDiGetClassDevsW failed: %v", lastErrno())
	}
	defer procSetupDiDestroyDeviceInfoList_HID.Call(hDevInfo)

	var out []LamzuDeviceInfo
	for idx := 0; ; idx++ {
		var ifData SP_DEVICE_INTERFACE_DATA
		ifData.CbSize = uint32(unsafe.Sizeof(ifData))

		r1, _, eEnum := procSetupDiEnumDeviceInterfaces_HID.Call(
			hDevInfo, 0,
			uintptr(unsafe.Pointer(&g)),
			uintptr(idx),
			uintptr(unsafe.Pointer(&ifData)),
		)
		if r1 == 0 {
			if errno, ok := eEnum.(syscall.Errno); ok && errno == ERROR_NO_MORE_ITEMS {
				break
			}
			break
		}

		var required uint32
		procSetupDiGetDeviceInterfaceDetailW_HID.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&ifData)),
			0, 0,
			uintptr(unsafe.Pointer(&required)),
			0,
		)
		if required == 0 {
			continue
		}

		buf := make([]byte, required)
		*(*uint32)(unsafe.Pointer(&buf[0])) = detailCbSizeW()

		r2, _, _ := procSetupDiGetDeviceInterfaceDetailW_HID.Call(
			hDevInfo,
			uintptr(unsafe.Pointer(&ifData)),
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(required),
			uintptr(unsafe.Pointer(&required)),
			0,
		)
		if r2 == 0 {
			continue
		}

		pathPtr := (*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(&buf[0])) + detailDevicePathOffset))
		path := utf16FromPtr(pathPtr)
		if path == "" {
			continue
		}

		info, ok := queryDeviceInfo(path)
		if !ok {
			continue
		}

		// 你日志里就是 0x37b0:0x0010，硬匹配最稳
		if info.VID == 0x37b0 && info.PID == 0x0010 {
			out = append(out, info)
		}
	}
	return out, nil
}

// ★强制：只允许 mi_02（对应 interface 2）
func SelectLamzuControlPath() (LamzuDeviceInfo, error) {
	ds, err := EnumerateLamzuDevices()
	if err != nil {
		return LamzuDeviceInfo{}, err
	}
	if len(ds) == 0 {
		return LamzuDeviceInfo{}, fmt.Errorf("no INCA device found")
	}

	for _, d := range ds {
		lp := strings.ToLower(d.Path)
		if strings.Contains(lp, "mi_02") && !strings.HasSuffix(lp, `\kbd`) {
			log.Printf("[DEV] 强制选择 INCA 控制通道 mi_02: %s (FeatureLen=%d)", d.Path, int(d.FeatureLen))
			return d, nil
		}
	}
	return LamzuDeviceInfo{}, fmt.Errorf("no mi_02 path found; cannot target interface 2")
}

func FindOneLamzuDevice() (LamzuDeviceInfo, error) {
	return SelectLamzuControlPath()
}

// ===== INCA 报文构造（来自抓包） =====

// 通用 CMD/VAL payload（抓包的 64B）：00 00 02 02 01 [CMD] 01 [VAL] ...
// 注意：若 FeatureLen=65，则 buf[0] 为 ReportID(0)，payload 从 buf[1] 开始写
func buildIncaCmdVal(cmd, val byte, featureLen int) []byte {
	// 抓包 payload 固定 64B
	const payloadLen = 64
	if featureLen <= 0 {
		featureLen = payloadLen
	}
	if featureLen < payloadLen {
		featureLen = payloadLen
	}

	buf := make([]byte, featureLen)

	off := 0
	// FeatureReportByteLength 含 ReportID 字节时（你这里是 65）
	if featureLen == payloadLen+1 {
		buf[0] = 0x00 // ReportID=0
		off = 1
	}

	// payload 写入（严格按抓包）
	buf[off+0] = 0x00
	buf[off+1] = 0x00
	buf[off+2] = 0x02
	buf[off+3] = 0x02
	buf[off+4] = 0x01
	buf[off+5] = cmd
	buf[off+6] = 0x01
	buf[off+7] = val
	// 其余自动为 0
	return buf
}

// 睡眠 payload（抓包的 64B）：00 00 02 03 00 07 01 [hi] [lo] ...
// 秒数大端：seconds = hi*256 + lo（你抓包验证了 1800=0x0708，600=0x0258 等）[2](https://maynoothuniversity-my.sharepoint.com/personal/shengwei_huang_2022_mumail_ie/Documents/Microsoft%20Copilot%20Chat%20Files/%E7%8E%B0%E5%9C%A8%E7%9A%84%E4%BB%A3%E7%A0%81.txt)[3](https://maynoothuniversity-my.sharepoint.com/personal/shengwei_huang_2022_mumail_ie/Documents/Microsoft%20Copilot%20Chat%20Files/INCA%E6%8A%93%E5%8C%85%E7%AD%9B%E9%80%89%E7%BB%93%E6%9E%9C-%E5%88%86%E6%AE%B5.txt)
func buildIncaSleep(sleepSec int, featureLen int) []byte {
	const payloadLen = 64
	if featureLen <= 0 {
		featureLen = payloadLen
	}
	if featureLen < payloadLen {
		featureLen = payloadLen
	}

	if sleepSec < 0 {
		sleepSec = 0
	}
	if sleepSec > 65535 {
		sleepSec = 65535
	}
	hi := byte((sleepSec >> 8) & 0xFF)
	lo := byte(sleepSec & 0xFF)

	buf := make([]byte, featureLen)

	off := 0
	if featureLen == payloadLen+1 {
		buf[0] = 0x00 // ReportID=0
		off = 1
	}

	// payload 按抓包
	buf[off+0] = 0x00
	buf[off+1] = 0x00
	buf[off+2] = 0x02
	buf[off+3] = 0x03
	buf[off+4] = 0x00
	buf[off+5] = 0x07
	buf[off+6] = 0x01
	buf[off+7] = hi
	buf[off+8] = lo
	return buf
}

func dump12(tag string, b []byte) {
	// 如果长度是 65，跳过第 0 字节 ReportID，打印 payload 的前 12 字节
	start := 0
	if len(b) == 65 {
		start = 1
	}
	n := 12
	if len(b)-start < n {
		n = len(b) - start
	}
	log.Printf("%s %x", tag, b[start:start+n])
}

func ApplyLamzuSetting(path string, perf PerfMode, poll PollingRate, motionSync bool, sleepSec int) error {
	dev, err := FindOneLamzuDevice()
	if err == nil && dev.Path != "" {
		path = dev.Path
	}

	// 用 FeatureLen（取不到就 64）；写后回读也用同样长度
	flen := int(dev.FeatureLen)
	if flen <= 0 {
		flen = 64
	}

	// 1) polling
	yy, err := pollingToYY(poll) // 1000->0x01,2000->0x20... [1](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
	if err != nil {
		return err
	}
	req := buildIncaCmdVal(0x00, yy, flen)
	dump12("[SEND poll ]", req)
	if err := sendFeatureReport(path, req); err != nil {
		return fmt.Errorf("poll set failed: %w", err)
	}
	time.Sleep(20 * time.Millisecond)
	if resp, e := getFeature(path, 0x00, flen); e == nil {
		dump12("[READ poll ]", resp)
	}

	// 2) motion sync
	msVal := byte(0x00)
	if motionSync {
		msVal = 0x01
	}
	req = buildIncaCmdVal(0x09, msVal, flen) // CMD=0x09 [1](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
	dump12("[SEND ms   ]", req)
	if err := sendFeatureReport(path, req); err != nil {
		return fmt.Errorf("motion sync set failed: %w", err)
	}
	time.Sleep(20 * time.Millisecond)
	if resp, e := getFeature(path, 0x00, flen); e == nil {
		dump12("[READ ms   ]", resp)
	}

	// 3) perf mode (0x0B + 0x13) [1](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
	switch perf {
	case PerfOffice:
		req = buildIncaCmdVal(0x0B, 0x00, flen)
		dump12("[SEND 0B   ]", req)
		if err := sendFeatureReport(path, req); err != nil {
			return fmt.Errorf("office 0x0B failed: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
		req = buildIncaCmdVal(0x13, 0x00, flen)
		dump12("[SEND 13   ]", req)
		if err := sendFeatureReport(path, req); err != nil {
			return fmt.Errorf("office 0x13 failed: %w", err)
		}
	case PerfSpeed:
		req = buildIncaCmdVal(0x0B, 0x01, flen)
		dump12("[SEND 0B   ]", req)
		if err := sendFeatureReport(path, req); err != nil {
			return fmt.Errorf("speed 0x0B failed: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
		req = buildIncaCmdVal(0x13, 0x00, flen)
		dump12("[SEND 13   ]", req)
		if err := sendFeatureReport(path, req); err != nil {
			return fmt.Errorf("speed 0x13 failed: %w", err)
		}
	case Perf20000FPS:
		req = buildIncaCmdVal(0x0B, 0x01, flen)
		dump12("[SEND 0B   ]", req)
		if err := sendFeatureReport(path, req); err != nil {
			return fmt.Errorf("20000fps 0x0B failed: %w", err)
		}
		time.Sleep(20 * time.Millisecond)
		req = buildIncaCmdVal(0x13, 0x01, flen)
		dump12("[SEND 13   ]", req)
		if err := sendFeatureReport(path, req); err != nil {
			return fmt.Errorf("20000fps 0x13 failed: %w", err)
		}
	default:
		return fmt.Errorf("unknown perf mode: %v", perf)
	}
	time.Sleep(20 * time.Millisecond)
	if resp, e := getFeature(path, 0x00, flen); e == nil {
		dump12("[READ perf ]", resp)
	}

	// 4) sleep cmd=0x07 hi lo (big-endian) [2](https://maynoothuniversity-my.sharepoint.com/personal/shengwei_huang_2022_mumail_ie/Documents/Microsoft%20Copilot%20Chat%20Files/%E7%8E%B0%E5%9C%A8%E7%9A%84%E4%BB%A3%E7%A0%81.txt)[3](https://maynoothuniversity-my.sharepoint.com/personal/shengwei_huang_2022_mumail_ie/Documents/Microsoft%20Copilot%20Chat%20Files/INCA%E6%8A%93%E5%8C%85%E7%AD%9B%E9%80%89%E7%BB%93%E6%9E%9C-%E5%88%86%E6%AE%B5.txt)
	req = buildIncaSleep(sleepSec, flen)
	dump12("[SEND sleep]", req)
	if err := sendFeatureReport(path, req); err != nil {
		return fmt.Errorf("sleep set failed: %w", err)
	}
	time.Sleep(20 * time.Millisecond)
	if resp, e := getFeature(path, 0x00, flen); e == nil {
		dump12("[READ sleep]", resp)
	}

	return nil
}
