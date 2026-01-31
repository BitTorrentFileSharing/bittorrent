// Package logger provides JSON line logging to stdout.
package logger

import (
	"encoding/json"
	"maps"
	"os"
	"sync"
	"time"
)

var enc = json.NewEncoder(os.Stdout)
var mu sync.Mutex

// Log emits JSON lines to Stdout with event name and key-value pairs.
func Log(event string, kv map[string]any) {
	kvCopy := make(map[string]any, len(kv)+2)
	maps.Copy(kvCopy, kv)
	kvCopy["event"] = event
	kvCopy["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	mu.Lock()
	_ = enc.Encode(kvCopy)
	mu.Unlock()
}
