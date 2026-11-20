package gomobile

import (
	"context"
	"sync"
	"time"

	"github.com/inkweave/inkweave/internal/server"
)

// EngineInfo contains flat primitive fields exported across the gomobile Java/Kotlin bridge.
type EngineInfo struct {
	Port    int
	Token   string
	BaseURL string
}

var (
	mu     sync.Mutex
	active *server.Engine
)

// StartEngine launches the embedded pure-Go Inkweave engine bound to 127.0.0.1 on an ephemeral port.
func StartEngine(dataDir string) (*EngineInfo, error) {
	mu.Lock()
	defer mu.Unlock()

	if active != nil {
		return &EngineInfo{
			Port:    active.Port,
			Token:   active.Token,
			BaseURL: active.BaseURL,
		}, nil
	}

	eng, err := server.StartEngine("127.0.0.1", 0, dataDir)
	if err != nil {
		return nil, err
	}
	active = eng
	return &EngineInfo{
		Port:    eng.Port,
		Token:   eng.Token,
		BaseURL: eng.BaseURL,
	}, nil
}

// StopEngine gracefully stops the embedded HTTP server and closes SQLite indexes.
func StopEngine() error {
	mu.Lock()
	defer mu.Unlock()

	if active == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := active.Shutdown(ctx)
	active = nil
	return err
}

// Version returns the Inkweave engine semantic version.
func Version() string {
	return server.Version
}
