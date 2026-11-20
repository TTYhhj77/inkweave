package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/inkweave/inkweave/internal/server"
)

func main() {
	configureAndroidDNS()

	host := flag.String("host", "127.0.0.1", "Bind host (use 0.0.0.0 for LAN access)")
	port := flag.Int("port", 7842, "HTTP port (0 = auto-select free port)")
	dataDir := flag.String("data-dir", "", "Custom directory for Inkweave config and novel repositories")
	readyFile := flag.String("ready-file", "", "Optional JSON file path to write bound port/token/url once ready")
	noOpen := flag.Bool("no-open", false, "Do not automatically open browser window")
	showVer := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVer {
		fmt.Printf("inkweave %s\n", server.Version)
		return
	}

	eng, err := server.StartEngine(*host, *port, *dataDir)
	if err != nil {
		log.Fatalf("Failed to start Inkweave engine: %v", err)
	}

	if *readyFile != "" {
		payload, _ := json.Marshal(map[string]any{
			"port":     eng.Port,
			"token":    eng.Token,
			"base_url": eng.BaseURL,
		})
		tmp := *readyFile + ".tmp"
		if err := os.WriteFile(tmp, payload, 0o600); err == nil {
			_ = os.Rename(tmp, *readyFile)
		}
	}

	fmt.Printf("\n  Inkweave Literary Studio v%s\n", server.Version)
	fmt.Printf("  Workspace : %s\n", eng.Workspace.BaseDir())
	fmt.Printf("  Studio URL: %s\n\n", eng.BaseURL)

	if !*noOpen {
		go openBrowser(eng.BaseURL)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down Inkweave cleanly...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = eng.Shutdown(ctx)
}

// configureAndroidDNS enables pure-Go DNS resolution on Android where /etc/resolv.conf does not exist.
func configureAndroidDNS() {
	raw := strings.TrimSpace(os.Getenv("INKWEAVE_DNS_SERVERS"))
	if raw == "" {
		return
	}
	var servers []string
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		if !strings.Contains(p, ":") {
			p = net.JoinHostPort(p, "53")
		}
		servers = append(servers, p)
	}
	if len(servers) == 0 {
		return
	}
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := net.Dialer{Timeout: 4 * time.Second}
			var lastErr error
			for _, srv := range servers {
				conn, err := d.DialContext(ctx, network, srv)
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			return nil, lastErr
		},
	}
}

func openBrowser(url string) {
	time.Sleep(300 * time.Millisecond)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}
