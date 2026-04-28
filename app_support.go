package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Applied struct {
	perf       PerfMode
	poll       PollingRate
	motionSync bool
	sleepSec   int
	ok         bool
}

func exeDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(exe)
}

func printBanner(cfgPath string) {
	log.Printf("========================================")
	log.Printf(" %s (Console)", appDisplayName)
	log.Printf(" Config: %s", cfgPath)
	log.Printf("========================================")
}

func printConfig(cfg *Config) {
	log.Printf("[CFG] interval=%s", cfg.Interval)
	log.Printf("[CFG] hit    : mode=%s poll=%dHz ms=%v sleep=%ds", perfName(cfg.HitMode), cfg.HitPoll, cfg.HitMotionSync, cfg.HitSleepSec)
	log.Printf("[CFG] default: mode=%s poll=%dHz ms=%v sleep=%ds", perfName(cfg.DefaultMode), cfg.DefaultPoll, cfg.DefaultMotionSync, cfg.DefaultSleepSec)
	log.Printf("[CFG] whitelist(%d): %s", len(cfg.Whitelist), strings.Join(cfg.Whitelist, ", "))
}

func waitForever() {
	log.Printf("Press Ctrl+C to exit.")
	select {}
}

func tickOnce(cfg *Config, last *Applied) (switchMsg string, errStr string, devCount int, logicalCount int) {
	proc, err := ForegroundProcessName()
	if err != nil {
		return "", "", 0, 0
	}
	proc = strings.ToLower(filepath.Base(proc))

	_, hit := cfg.WhitelistSet[proc]

	wantPerf := cfg.DefaultMode
	wantPoll := cfg.DefaultPoll
	wantMS := cfg.DefaultMotionSync
	wantSleep := cfg.DefaultSleepSec
	if hit {
		wantPerf = cfg.HitMode
		wantPoll = cfg.HitPoll
		wantMS = cfg.HitMotionSync
		wantSleep = cfg.HitSleepSec
	}

	devs := FindAllLamzuDevices()
	logicalCount = len(devs)
	devCount = CountUniqueLamzuDevices(devs)
	if last.ok && last.perf == wantPerf && last.poll == wantPoll && last.motionSync == wantMS && last.sleepSec == wantSleep {
		return "", "", devCount, logicalCount
	}

	dev, findErr := FindOneLamzuDevice()
	if findErr != nil {
		return "", "未找到可用 LAMZU 设备: " + findErr.Error(), devCount, logicalCount
	}

	if err := ApplyLamzuSetting(dev.Path, wantPerf, wantPoll, wantMS, wantSleep); err != nil {
		return "", "应用设置失败: " + err.Error(), devCount, logicalCount
	}

	*last = Applied{perf: wantPerf, poll: wantPoll, motionSync: wantMS, sleepSec: wantSleep, ok: true}
	PrintBatteryINCA(dev)
	if hit {
		return fmt.Sprintf("[SWITCH] 命中白名单(%s) -> %s + %dHz + MS=%v + Sleep=%ds", proc, perfName(wantPerf), wantPoll, wantMS, wantSleep), "", devCount, logicalCount
	}
	return fmt.Sprintf("[SWITCH] 未命中白名单(%s) -> %s + %dHz + MS=%v + Sleep=%ds", proc, perfName(wantPerf), wantPoll, wantMS, wantSleep), "", devCount, logicalCount
}

func enumerateDevices() {
	infos, enumErr := EnumerateLamzuDevices()
	if enumErr != nil {
		log.Printf("[DEV] 枚举 HID 设备失败: %v", enumErr)
		return
	}
	if len(infos) == 0 {
		log.Printf("[DEV] 未发现 LAMZU 设备。")
		return
	}

	log.Printf("[DEV] 发现 %d 个 LAMZU HID 逻辑接口:", len(infos))
	for i, d := range infos {
		log.Printf(" #%d Manufacturer=%q Product=%q VID=0x%04x PID=0x%04x Path=%s", i+1, d.Manufacturer, d.Product, d.VID, d.PID, d.Path)
	}
}

func handleError(lastErr *string, errStr string) {
	if errStr != "" && errStr != *lastErr {
		*lastErr = errStr
		log.Printf("[ERR] %s", errStr)
	} else if errStr == "" {
		*lastErr = ""
	}
}

func runConsoleApp() {
	log.SetFlags(log.LstdFlags)

	cfgPath := filepath.Join(exeDir(), configFileName)
	app, err := NewAutoSwitchApp(cfgPath)
	if err != nil {
		log.Printf("[ERR] %v", err)
		waitForever()
	}

	printBanner(cfgPath)
	printConfig(app.CurrentConfig())
	enumerateDevices()

	if err := app.Run(context.Background()); err != nil {
		log.Printf("[ERR] %v", err)
		waitForever()
	}
}
