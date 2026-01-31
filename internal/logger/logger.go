// Package logger provides JSON line logging to stdout.
package logger

import (
	"encoding/json"
	"maps"
	"os"
	"sync"
	"time"
)

const extraFieldsCount = 2

var (
	enc = json.NewEncoder(os.Stdout)
	mu  sync.Mutex
)

// Log emits JSON lines to Stdout with event name and key-value pairs.
func Log(event string, kv map[string]any) {
	kvCopy := make(map[string]any, len(kv)+extraFieldsCount)
	maps.Copy(kvCopy, kv)
	kvCopy["event"] = event
	kvCopy["ts"] = time.Now().UTC().Format(time.RFC3339Nano)

	mu.Lock()
	defer mu.Unlock()

	if err := enc.Encode(kvCopy); err != nil {
		// Fallback if JSON encoding fails
		_, _ = os.Stdout.WriteString(`{"event":"logger_error","error":"` + err.Error() + `"}\n`)
	}
}
