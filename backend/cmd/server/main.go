package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"sniping_engine/internal/config"
	"sniping_engine/internal/engine"
	"sniping_engine/internal/httpapi"
	"sniping_engine/internal/logbus"
	"sniping_engine/internal/notify"
	"sniping_engine/internal/provider/standard"
	"sniping_engine/internal/store/sqlite"
	"sniping_engine/internal/utils"
)

func main() {
	configPath := flag.String("config", "./config.yaml", "path to config.yaml")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	bus := logbus.New(200)
	stopConsole := startConsoleLogger(bus)
	defer stopConsole()

	ctx := context.Background()
	store, err := sqlite.Open(ctx, cfg.Storage.SQLitePath)
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	defer store.Close()

	if v, ok, err := store.GetLimitsSettings(ctx); err == nil && ok {
		if v.MaxPerTargetInFlight > 0 {
			cfg.Limits.MaxPerTargetInFlight = v.MaxPerTargetInFlight
		}
		if v.CaptchaMaxInFlight > 0 {
			cfg.Limits.CaptchaMaxInFlight = v.CaptchaMaxInFlight
		}
	} else if err != nil {
		bus.Log("warn", "读取并发设置失败", map[string]any{"error": err.Error()})
	}

	utils.SetCaptchaMaxConcurrent(cfg.Limits.CaptchaMaxInFlight)

	prov := standard.New(cfg.Provider, cfg.Proxy, bus)
	emailNotifier := notify.NewEmailNotifier(store, bus)
	eng := engine.New(engine.Options{
		Store:    store,
		Provider: prov,
		Bus:      bus,
		Limits:   cfg.Limits,
		Task:     cfg.Task,
		Notifier: emailNotifier,
	})

	api := httpapi.New(httpapi.Options{
		Cfg:      cfg,
		Bus:      bus,
		Store:    store,
		Engine:   eng,
		Notifier: emailNotifier,
	})

	server := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           api.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	serverErr := make(chan error, 1)

	ln, err := net.Listen("tcp", cfg.Server.Addr)
	if err != nil {
		bus.Log("error", "监听端口失败", map[string]any{"addr": cfg.Server.Addr, "error": err.Error()})
		return
	}
	hostPort := displayHostPortFromListener(ln, cfg.Server.Addr)
	printStartupBanner(cfg, *configPath, hostPort)
	bus.Log("info", "服务启动中", map[string]any{"addr": ln.Addr().String()})
	bus.Log("info", "服务已启动，开始监听", map[string]any{"addr": ln.Addr().String()})

	go func() {
		serverErr <- server.Serve(ln)
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	select {
	case sig := <-stop:
		bus.Log("info", "收到退出信号，正在停止服务", map[string]any{"signal": sig.String()})
	case err := <-serverErr:
		if err != nil && err != http.ErrServerClosed {
			bus.Log("error", "服务异常", map[string]any{"error": err.Error()})
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	_ = eng.StopAll(shutdownCtx)
	_ = emailNotifier.Close(shutdownCtx)
	_ = server.Shutdown(shutdownCtx)
	bus.Log("info", "服务已停止", nil)
}

func startConsoleLogger(bus *logbus.Bus) func() {
	if bus == nil {
		return func() {}
	}

	showDebug := strings.EqualFold(strings.TrimSpace(os.Getenv("SNIPING_ENGINE_DEBUG")), "1") ||
		strings.EqualFold(strings.TrimSpace(os.Getenv("SNIPING_ENGINE_DEBUG")), "true")

	ch, cancel := bus.Subscribe(256)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for msg := range ch {
			if msg.Type != "log" {
				continue
			}
			data, ok := msg.Data.(logbus.LogData)
			if !ok {
				continue
			}
			level := strings.ToLower(strings.TrimSpace(data.Level))
			if level == "debug" && !showDebug {
				continue
			}

			ts := time.UnixMilli(msg.Time).Format("2006-01-02 15:04:05.000")
			lv := strings.ToUpper(level)
			if lv == "" {
				lv = "INFO"
			}
			line := fmt.Sprintf("%s %-5s %s", ts, lv, strings.TrimSpace(data.Msg))
			if len(data.Fields) > 0 {
				if b, err := json.Marshal(data.Fields); err == nil && len(b) > 0 {
					line += " " + string(b)
				}
			}
			fmt.Println(line)
		}
	}()

	return func() {
		cancel()
	<-done
	}
}

func printStartupBanner(cfg config.Config, configPath string, hostPort string) {
	absCfg := strings.TrimSpace(configPath)
	if p, err := filepath.Abs(configPath); err == nil {
		absCfg = p
	}
	fmt.Println("============================================================")
	fmt.Println("sniping_engine backend")
	fmt.Println("------------------------------------------------------------")
	fmt.Printf("Config    : %s\n", absCfg)
	fmt.Printf("Listen    : http://%s\n", hostPort)
	fmt.Printf("Health    : http://%s/health\n", hostPort)
	fmt.Printf("WebSocket : ws://%s/ws\n", hostPort)
	if strings.TrimSpace(cfg.Provider.BaseURL) != "" {
		fmt.Printf("Upstream  : %s\n", strings.TrimSpace(cfg.Provider.BaseURL))
	}
	if strings.TrimSpace(cfg.Proxy.Global) != "" {
		fmt.Printf("Proxy     : %s\n", strings.TrimSpace(cfg.Proxy.Global))
	}
	if strings.TrimSpace(cfg.Storage.SQLitePath) != "" {
		fmt.Printf("SQLite    : %s\n", strings.TrimSpace(cfg.Storage.SQLitePath))
	}
	fmt.Println("------------------------------------------------------------")
	if strings.TrimSpace(os.Getenv("SNIPING_ENGINE_DEBUG")) == "" {
		fmt.Println("Tip       : set SNIPING_ENGINE_DEBUG=1 to show debug logs")
	}
	fmt.Println("============================================================")
}

func displayHostPortFromListener(ln net.Listener, cfgAddr string) string {
	if ln != nil {
		if ta, ok := ln.Addr().(*net.TCPAddr); ok {
			port := ta.Port
			ip := ta.IP
			if ip == nil || ip.To4() == nil {
				return net.JoinHostPort("::1", fmt.Sprint(port))
			}
			return net.JoinHostPort("127.0.0.1", fmt.Sprint(port))
		}
	}

	addr := strings.TrimSpace(cfgAddr)
	if addr == "" {
		return "127.0.0.1:8090"
	}
	if strings.HasPrefix(addr, ":") {
		return net.JoinHostPort("127.0.0.1", strings.TrimPrefix(addr, ":"))
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" {
		host = "127.0.0.1"
	}
	if host == "::" {
		host = "::1"
	}
	return net.JoinHostPort(host, port)
}
