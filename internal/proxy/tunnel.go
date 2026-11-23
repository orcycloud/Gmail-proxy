package proxy

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// Tunnel 定义一条本地监听 -> 上游目标的转发规则
type Tunnel struct {
	Name          string        `yaml:"name"`
	Listen        string        `yaml:"listen"`          // 本地监听地址，如 ":995"
	Upstream      string        `yaml:"upstream"`        // 上游地址，如 "pop.gmail.com:995"
	Timeout       time.Duration `yaml:"timeout"`         // 单连接空闲超时，默认 5 分钟
	OutboundProxy string        `yaml:"outbound_proxy"`  // 出站 HTTP 代理 "ip:port"，空则直连
	OutboundAuth  string        `yaml:"outbound_auth"`   // 出站代理认证 "user:pass"，可选
}

// Server 代理服务，管理多条 Tunnel
type Server struct {
	Tunnels []Tunnel

	listeners []net.Listener
	wg        sync.WaitGroup
	mu        sync.Mutex
}

// NewServer 构造 Server
func NewServer(tunnels []Tunnel) *Server {
	s := &Server{Tunnels: tunnels}
	for i := range s.Tunnels {
		if s.Tunnels[i].Timeout == 0 {
			s.Tunnels[i].Timeout = 5 * time.Minute
		}
	}
	return s
}

// Run 启动所有 Tunnel，阻塞直到 ctx 取消
func (s *Server) Run(ctx context.Context) error {
	s.mu.Lock()
	s.listeners = make([]net.Listener, 0, len(s.Tunnels))
	s.mu.Unlock()

	for _, t := range s.Tunnels {
		ln, err := net.Listen("tcp", t.Listen)
		if err != nil {
			log.Printf("[%s] 监听 %s 失败: %v", t.Name, t.Listen, err)
			return err
		}
		s.mu.Lock()
		s.listeners = append(s.listeners, ln)
		s.mu.Unlock()

		if t.OutboundProxy != "" {
			log.Printf("[%s] 已启动  %s  ->  (HTTP CONNECT %s)  ->  %s", t.Name, t.Listen, t.OutboundProxy, t.Upstream)
		} else {
			log.Printf("[%s] 已启动  %s  ->  %s", t.Name, t.Listen, t.Upstream)
		}

		s.wg.Add(1)
		go s.acceptLoop(ctx, ln, t)
	}

	<-ctx.Done()
	log.Println("正在关闭所有监听...")
	s.mu.Lock()
	for _, ln := range s.listeners {
		_ = ln.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	log.Println("所有连接已退出，服务已停止")
	return nil
}

func (s *Server) acceptLoop(ctx context.Context, ln net.Listener, t Tunnel) {
	defer s.wg.Done()

	for {
		conn, err := ln.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			log.Printf("[%s] Accept 错误: %v", t.Name, err)
			continue
		}

		s.wg.Add(1)
		go func(c net.Conn) {
			defer s.wg.Done()
			defer c.Close()
			s.handleConn(ctx, c, t)
		}(conn)
	}
}

func (s *Server) handleConn(ctx context.Context, client net.Conn, t Tunnel) {
	upstream, err := dialUpstream(t)
	if err != nil {
		log.Printf("[%s] 连接上游失败: %v (来自 %s)", t.Name, err, client.RemoteAddr())
		return
	}
	defer upstream.Close()

	log.Printf("[%s] 新连接  %s  <->  %s", t.Name, client.RemoteAddr(), t.Upstream)

	done := make(chan struct{}, 2)

	go func() {
		_, _ = copyWithTimeout(upstream, client, t.Timeout)
		done <- struct{}{}
	}()

	go func() {
		_, _ = copyWithTimeout(client, upstream, t.Timeout)
		done <- struct{}{}
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
	log.Printf("[%s] 连接结束  %s", t.Name, client.RemoteAddr())
}

// dialUpstream 根据配置选择直连或经 HTTP CONNECT 隧道
func dialUpstream(t Tunnel) (net.Conn, error) {
	if t.OutboundProxy == "" {
		return net.DialTimeout("tcp", t.Upstream, 10*time.Second)
	}
	return dialViaHTTPConnect(t.OutboundProxy, t.Upstream, t.OutboundAuth)
}

// dialViaHTTPConnect 连接 HTTP 代理并发送 CONNECT 请求
// 返回的 net.Conn 是与目标之间的隧道，之后调用者可以直接转发数据
func dialViaHTTPConnect(proxyAddr, target, auth string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxyAddr, 10*time.Second)
	if err != nil {
		return nil, fmt.Errorf("连 HTTP 代理 %s 失败: %w", proxyAddr, err)
	}

	// 发送 CONNECT 请求
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", target, target)
	if auth != "" {
		cred := base64.StdEncoding.EncodeToString([]byte(auth))
		req += fmt.Sprintf("Proxy-Authorization: Basic %s\r\n", cred)
	}
	req += "Proxy-Connection: Keep-Alive\r\n\r\n"

	if _, err := io.WriteString(conn, req); err != nil {
		conn.Close()
		return nil, fmt.Errorf("写 CONNECT 请求失败: %w", err)
	}

	// 读响应
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, nil)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("读代理响应失败: %w", err)
	}
	// http.ReadResponse 会把 body 读掉，但 CONNECT 隧道没有 body
	// 需要关闭 resp.Body（实际上是空的）
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("代理拒绝 CONNECT: %s", resp.Status)
	}

	return conn, nil
}

// copyWithTimeout 类似 io.Copy，但在指定时间内没有数据读写时返回
func copyWithTimeout(dst, src net.Conn, timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		return io.Copy(dst, src)
	}

	var written int64
	buf := make([]byte, 32*1024)

	for {
		_ = src.SetReadDeadline(time.Now().Add(timeout))
		nr, err := src.Read(buf)
		if nr > 0 {
			_, werr := dst.Write(buf[:nr])
			if werr != nil {
				return written, werr
			}
			written += int64(nr)
			_ = dst.SetReadDeadline(time.Time{})
			_ = src.SetReadDeadline(time.Time{})
		}
		if err != nil {
			if err == io.EOF {
				return written, nil
			}
			return written, err
		}
	}
}
