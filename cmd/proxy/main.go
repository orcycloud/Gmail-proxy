package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"gmail-proxy/internal/proxy"
)

func main() {
	configPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	flag.Parse()

	cfg, err := loadConfig(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	if len(cfg.Tunnels) == 0 {
		log.Fatal("配置中没有任何 tunnel 规则")
	}

	srv := proxy.NewServer(cfg.Tunnels)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		log.Printf("收到信号 %v，开始优雅关闭...", sig)
		cancel()
	}()

	fmt.Println("==========================================")
	fmt.Println("  Gmail Proxy - TLS 透传代理")
	fmt.Println("==========================================")
	for _, t := range cfg.Tunnels {
		fmt.Printf("  %-6s  %s  ->  %s\n", t.Name, t.Listen, t.Upstream)
	}
	fmt.Println("------------------------------------------")
	fmt.Println("按 Ctrl+C 退出")
	fmt.Println()

	if err := srv.Run(ctx); err != nil {
		log.Fatalf("服务异常退出: %v", err)
	}
}
