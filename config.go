package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const configFileName = "lamzu_autoswitch.conf"

// ======== LAMZU INCA 模式 ========
type PerfMode byte

const (
	PerfOffice   PerfMode = 0 // 办公：0x0B=0, 0x13=0 [2](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
	PerfSpeed    PerfMode = 1 // 高速：0x0B=1, 0x13=0 [2](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
	Perf20000FPS PerfMode = 2 // 20000FPS：0x0B=1, 0x13=1 [2](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
)

type PollingRate int

const (
	Poll1000 PollingRate = 1000
	Poll2000 PollingRate = 2000
	Poll4000 PollingRate = 4000
	Poll8000 PollingRate = 8000
)

type Config struct {
	Interval time.Duration

	// 命中白名单
	HitMode       PerfMode
	HitPoll       PollingRate
	HitMotionSync bool
	HitSleepSec   int

	// 未命中（默认）
	DefaultMode       PerfMode
	DefaultPoll       PollingRate
	DefaultMotionSync bool
	DefaultSleepSec   int

	Whitelist    []string
	WhitelistSet map[string]struct{}
	ConfigPath   string
}

var configWriteMu sync.Mutex

func defaultConfigText() string {
	return `# LAMZU INCA AutoSwitch 配置文件
# ------------------------------------------------
# 说明：
# 1) 以 key=value 配置策略
# 2) 其余非空、非 # 开头的行，会被当作“白名单程序名”（每行一个，例如 cs2.exe）
#
# 可配置项：
# interval_seconds=60
#
# default_mode=office|speed|20000fps
# default_poll=1000|2000|4000|8000
# default_motion_sync=0|1|true|false|on|off
# default_sleep_seconds=30
#
# hit_mode=office|speed|20000fps
# hit_poll=1000|2000|4000|8000
# hit_motion_sync=0|1|true|false|on|off
# hit_sleep_seconds=600
#
# ------------------------------------------------
interval_seconds=60

# 默认：1000Hz + MS Off + 办公 + 睡眠30秒
default_mode=office
default_poll=1000
default_motion_sync=off
default_sleep_seconds=30

# 命中白名单：2000Hz + MS Off + 20000FPS + 睡眠10分钟
hit_mode=20000fps
hit_poll=2000
hit_motion_sync=off
hit_sleep_seconds=600

# 白名单示例（每行一个进程名）：
# cs2.exe
# valorant.exe
`
}

func ensureConfigExists(path string) error {
	_, err := os.Stat(path)
	if err == nil {
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(defaultConfigText()), 0644)
}

func cloneConfig(src *Config) *Config {
	if src == nil {
		return nil
	}
	dst := *src
	dst.Whitelist = append([]string(nil), src.Whitelist...)
	dst.WhitelistSet = make(map[string]struct{}, len(src.WhitelistSet))
	for k := range src.WhitelistSet {
		dst.WhitelistSet[k] = struct{}{}
	}
	return &dst
}

func saveConfig(path string, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}

	configWriteMu.Lock()
	defer configWriteMu.Unlock()

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(formatConfig(cfg)), 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func formatConfig(cfg *Config) string {
	var b strings.Builder
	b.WriteString("# LAMZU INCA AutoSwitch configuration\n")
	b.WriteString("# Updates made from the tray GUI are written back immediately.\n\n")
	fmt.Fprintf(&b, "interval_seconds=%d\n\n", int(cfg.Interval/time.Second))
	fmt.Fprintf(&b, "default_mode=%s\n", perfName(cfg.DefaultMode))
	fmt.Fprintf(&b, "default_poll=%d\n", cfg.DefaultPoll)
	fmt.Fprintf(&b, "default_motion_sync=%s\n", onOff(cfg.DefaultMotionSync))
	fmt.Fprintf(&b, "default_sleep_seconds=%d\n\n", cfg.DefaultSleepSec)
	fmt.Fprintf(&b, "hit_mode=%s\n", perfName(cfg.HitMode))
	fmt.Fprintf(&b, "hit_poll=%d\n", cfg.HitPoll)
	fmt.Fprintf(&b, "hit_motion_sync=%s\n", onOff(cfg.HitMotionSync))
	fmt.Fprintf(&b, "hit_sleep_seconds=%d\n\n", cfg.HitSleepSec)
	b.WriteString("# whitelist process names, one per line\n")
	for _, proc := range cfg.Whitelist {
		proc = strings.TrimSpace(proc)
		if proc == "" {
			continue
		}
		b.WriteString(proc)
		b.WriteByte('\n')
	}
	return b.String()
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func loadConfig(path string) (*Config, time.Time, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, time.Time{}, err
	}

	cfg := &Config{
		Interval: 60 * time.Second,

		HitMode:       Perf20000FPS,
		HitPoll:       Poll2000,
		HitMotionSync: false,
		HitSleepSec:   600,

		DefaultMode:       PerfOffice,
		DefaultPoll:       Poll1000,
		DefaultMotionSync: false,
		DefaultSleepSec:   30,

		Whitelist:    []string{},
		WhitelistSet: map[string]struct{}{},
		ConfigPath:   path,
	}

	f, err := os.Open(path)
	if err != nil {
		return nil, time.Time{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if i := strings.IndexByte(line, '='); i > 0 {
			key := strings.ToLower(strings.TrimSpace(line[:i]))
			val := strings.TrimSpace(line[i+1:])

			switch key {
			case "interval_seconds":
				sec, e := parseInt(val)
				if e != nil || sec <= 0 {
					return nil, time.Time{}, fmt.Errorf("invalid interval_seconds: %s", val)
				}
				cfg.Interval = time.Duration(sec) * time.Second

			case "hit_mode":
				m, e := parsePerf(val)
				if e != nil {
					return nil, time.Time{}, e
				}
				cfg.HitMode = m

			case "hit_poll":
				n, e := parseInt(val)
				if e != nil {
					return nil, time.Time{}, e
				}
				cfg.HitPoll = PollingRate(n)
				if _, e := pollingToYY(cfg.HitPoll); e != nil {
					return nil, time.Time{}, e
				}

			case "hit_motion_sync":
				b, e := parseBool(val)
				if e != nil {
					return nil, time.Time{}, e
				}
				cfg.HitMotionSync = b

			case "hit_sleep_seconds":
				n, e := parseInt(val)
				if e != nil || n < 0 || n > 65535 {
					return nil, time.Time{}, fmt.Errorf("invalid hit_sleep_seconds: %s", val)
				}
				cfg.HitSleepSec = n

			case "default_mode":
				m, e := parsePerf(val)
				if e != nil {
					return nil, time.Time{}, e
				}
				cfg.DefaultMode = m

			case "default_poll":
				n, e := parseInt(val)
				if e != nil {
					return nil, time.Time{}, e
				}
				cfg.DefaultPoll = PollingRate(n)
				if _, e := pollingToYY(cfg.DefaultPoll); e != nil {
					return nil, time.Time{}, e
				}

			case "default_motion_sync":
				b, e := parseBool(val)
				if e != nil {
					return nil, time.Time{}, e
				}
				cfg.DefaultMotionSync = b

			case "default_sleep_seconds":
				n, e := parseInt(val)
				if e != nil || n < 0 || n > 65535 {
					return nil, time.Time{}, fmt.Errorf("invalid default_sleep_seconds: %s", val)
				}
				cfg.DefaultSleepSec = n

			default:
				// 未知 key 忽略
			}
			continue
		}

		// 白名单行：只取 basename，转小写
		proc := strings.ToLower(filepath.Base(line))
		cfg.Whitelist = append(cfg.Whitelist, proc)
		cfg.WhitelistSet[proc] = struct{}{}
	}

	if err := sc.Err(); err != nil {
		return nil, time.Time{}, err
	}
	return cfg, fi.ModTime(), nil
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty int")
	}
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			return 0, fmt.Errorf("not int: %s", s)
		}
		n = n*10 + int(ch-'0')
	}
	return n, nil
}

func parseBool(s string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "on", "yes":
		return true, nil
	case "0", "false", "off", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool: %s", s)
	}
}

func parsePerf(s string) (PerfMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "office":
		return PerfOffice, nil
	case "speed":
		return PerfSpeed, nil
	case "20000fps":
		return Perf20000FPS, nil
	default:
		return 0, fmt.Errorf("unknown perf mode: %s", s)
	}
}

func perfName(p PerfMode) string {
	switch p {
	case PerfOffice:
		return "office"
	case PerfSpeed:
		return "speed"
	case Perf20000FPS:
		return "20000fps"
	default:
		return fmt.Sprintf("0x%02x", byte(p))
	}
}

// ======== INCA 回报率编码（抓包） ========
// 1000->0x01, 2000->0x20, 4000->0x40, 8000->0x80 [2](blob:https://m365.cloud.microsoft/cc5afcbc-93a5-4d76-9405-c2ed4901931e)
func pollingToYY(p PollingRate) (byte, error) {
	switch p {
	case Poll1000:
		return 0x01, nil
	case Poll2000:
		return 0x20, nil
	case Poll4000:
		return 0x40, nil
	case Poll8000:
		return 0x80, nil
	default:
		return 0, fmt.Errorf("unsupported polling rate: %d", p)
	}
}
