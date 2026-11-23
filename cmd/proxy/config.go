package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gmail-proxy/internal/proxy"
)

type configFile struct {
	Tunnels []proxy.Tunnel `yaml:"tunnels"`
}

// loadConfig 手写一个最小 YAML 解析器，避免引入第三方依赖
// 支持格式:
//   tunnels:
//     - name: pop3
//       listen: ":995"
//       upstream: "pop.gmail.com:995"
//       timeout: 5m
//       outbound_proxy: "10.0.0.1:10808"   # 可选，HTTP CONNECT 出站代理
//       outbound_auth:  "user:pass"        # 可选，Basic 认证
func loadConfig(path string) (*configFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取 %s 失败: %w", path, err)
	}

	cfg := &configFile{}
	lines := strings.Split(string(data), "\n")

	var current *proxy.Tunnel
	inTunnels := false

	for rawIdx, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		trimmed := strings.TrimSpace(line)

		// 跳过空行和注释
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		if strings.HasPrefix(trimmed, "tunnels:") {
			inTunnels = true
			continue
		}

		if !inTunnels {
			continue
		}

		// 新 tunnel 条目
		if strings.HasPrefix(trimmed, "- ") {
			if current != nil {
				cfg.Tunnels = append(cfg.Tunnels, *current)
			}
			current = &proxy.Tunnel{}
			kv := strings.TrimPrefix(trimmed, "- ")
			if k, v, ok := parseKV(kv); ok {
				applyKV(current, k, v)
			}
			continue
		}

		// 当前 tunnel 的属性
		if current != nil {
			if k, v, ok := parseKV(trimmed); ok {
				applyKV(current, k, v)
			} else {
				return nil, fmt.Errorf("第 %d 行无法解析: %q", rawIdx+1, raw)
			}
		}
	}

	if current != nil {
		cfg.Tunnels = append(cfg.Tunnels, *current)
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func parseKV(s string) (string, string, bool) {
	idx := strings.Index(s, ":")
	if idx < 0 {
		return "", "", false
	}
	k := strings.TrimSpace(s[:idx])
	v := strings.TrimSpace(s[idx+1:])
	// 去掉引号
	v = strings.Trim(v, `"'`)
	return k, v, true
}

func applyKV(t *proxy.Tunnel, key, val string) {
	switch strings.ToLower(key) {
	case "name":
		t.Name = val
	case "listen":
		t.Listen = val
	case "upstream":
		t.Upstream = val
	case "timeout":
		if d, err := time.ParseDuration(val); err == nil {
			t.Timeout = d
		}
	case "outbound_proxy", "proxy":
		t.OutboundProxy = val
	case "outbound_auth", "proxy_auth":
		t.OutboundAuth = val
	}
}

func validate(cfg *configFile) error {
	for i, t := range cfg.Tunnels {
		if t.Listen == "" {
			return fmt.Errorf("tunnel[%d] 缺少 listen", i)
		}
		if t.Upstream == "" {
			return fmt.Errorf("tunnel[%d] 缺少 upstream", i)
		}
	}
	if len(cfg.Tunnels) == 0 {
		return errors.New("没有定义任何 tunnel")
	}
	return nil
}
