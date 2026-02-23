package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	log.Printf(" LAMZU INCA AutoSwitch (Console)")
	log.Printf(" Config: %s", cfgPath)
	log.Printf("========================================")
}

func printConfig(cfg *Config) {
	log.Printf("[CFG] interval=%s", cfg.Interval)
	log.Printf("[CFG] hit    : mode=%s poll=%dHz ms=%v sleep=%ds",
		perfName(cfg.HitMode), cfg.HitPoll, cfg.HitMotionSync, cfg.HitSleepSec)
	log.Printf("[CFG] default: mode=%s poll=%dHz ms=%v sleep=%ds",
		perfName(cfg.DefaultMode), cfg.DefaultPoll, cfg.DefaultMotionSync, cfg.DefaultSleepSec)
	log.Printf("[CFG] whitelist(%d): %s", len(cfg.Whitelist), strings.Join(cfg.Whitelist, ", "))
}

func waitForever() {
	log.Printf("按 Ctrl+C 退出。")
	select {}
}

func tickOnce(cfg *Config, last *Applied) (string, string) {
	proc, err := ForegroundProcessName()
	if err != nil {
		return "", ""
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

	if last.ok && last.perf == wantPerf && last.poll == wantPoll && last.motionSync == wantMS && last.sleepSec == wantSleep {
		return "", ""
	}

	dev, findErr := FindOneLamzuDevice()
	if findErr != nil {
		return "", "未找到可用 INCA 设备：" + findErr.Error()
	}

	if err := ApplyLamzuSetting(dev.Path, wantPerf, wantPoll, wantMS, wantSleep); err != nil {
		return "", "应用设置失败：" + err.Error()
	}

	*last = Applied{perf: wantPerf, poll: wantPoll, motionSync: wantMS, sleepSec: wantSleep, ok: true}
	PrintBatteryINCA(dev)
	if hit {
		return fmt.Sprintf("[SWITCH] 命中白名单(%s) -> %s + %dHz + MS=%v + Sleep=%ds",
			proc, perfName(wantPerf), wantPoll, wantMS, wantSleep), ""
	}
	return fmt.Sprintf("[SWITCH] 未命中白名单(%s) -> %s + %dHz + MS=%v + Sleep=%ds",
		proc, perfName(wantPerf), wantPoll, wantMS, wantSleep), ""
}

func main() {
	log.SetFlags(log.LstdFlags)

	cfgPath := filepath.Join(exeDir(), configFileName)

	if err := ensureConfigExists(cfgPath); err != nil {
		log.Printf("[ERR] 无法创建配置文件：%v", err)
		waitForever()
	}

	cfg, modTime, err := loadConfig(cfgPath)
	if err != nil {
		log.Printf("[ERR] 读取配置失败：%v", err)
		waitForever()
	}

	printBanner(cfgPath)
	printConfig(cfg)

	infos, enumErr := EnumerateLamzuDevices()
	if enumErr != nil {
		log.Printf("[DEV] 枚举 HID 设备失败：%v", enumErr)
	} else {
		log.Printf("[DEV] 发现 %d 个 INCA HID 设备：", len(infos))
		for i, d := range infos {
			log.Printf("  #%d Manufacturer=%q Product=%q VID=0x%04x PID=0x%04x Path=%s",
				i+1, d.Manufacturer, d.Product, d.VID, d.PID, d.Path)
		}
	}

	// 低优先级（Windows 下有效；非 Windows stub）
	setLowPriorityDefaults(true, true)

	log.Printf("开始后台监控：每 %s 检查一次前台进程。", cfg.Interval)

	var last Applied
	var lastErr string

	for {
		// 热加载配置
		if fi, e := os.Stat(cfgPath); e == nil && fi.ModTime().After(modTime) {
			if nc, mt, e2 := loadConfig(cfgPath); e2 == nil {
				cfg, modTime = nc, mt
				log.Printf("[CFG] 检测到配置文件变更，已重新加载。")
				printConfig(cfg)
			}
		}

		msg, errStr := tickOnce(cfg, &last)
		if msg != "" {
			log.Print(msg)
		}
		if errStr != "" && errStr != lastErr {
			lastErr = errStr
			log.Printf("[ERR] %s", errStr)
		} else if errStr == "" {
			lastErr = ""
		}

		time.Sleep(cfg.Interval)
	}
}
