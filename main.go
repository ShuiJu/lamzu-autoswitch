//go:build legacyconsole

package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	log.SetFlags(log.LstdFlags)

	cfgPath := filepath.Join(exeDir(), configFileName)

	if err := ensureConfigExists(cfgPath); err != nil {
		log.Printf("[ERR] 无法创建配置文件: %v", err)
		waitForever()
	}

	cfg, modTime, err := loadConfig(cfgPath)
	if err != nil {
		log.Printf("[ERR] 读取配置失败: %v", err)
		waitForever()
	}

	printBanner(cfgPath)
	printConfig(cfg)
	enumerateDevices()

	setLowPriorityDefaults(true, true)

	log.Printf("开始后台监控：每 %s 检查一次前台进程。", cfg.Interval)

	var last Applied
	var lastErr string

	for {
		if fi, e := os.Stat(cfgPath); e == nil && fi.ModTime().After(modTime) {
			if nc, mt, e2 := loadConfig(cfgPath); e2 == nil {
				cfg, modTime = nc, mt
				log.Printf("[CFG] 检测到配置文件变更，已重新加载。")
				printConfig(cfg)
			}
		}

		msg, errStr, _, _ := tickOnce(cfg, &last)
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

var _ = fmt.Sprintf
