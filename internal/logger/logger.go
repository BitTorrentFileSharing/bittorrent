package logger

import (
	"encoding/json"
	"os"
	"sync"
	"time"
	"maps"
)

var enc = json.NewEncoder(os.Stdout)
var mu sync.Mutex

// Log emits json lines to Stdout
func Log(event string, kv map[string]any) {
	kvCopy := make(map[string]any, len(kv)+2)
	maps.Copy(kvCopy, kv)
	kvCopy["event"] = event
	kvCopy["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
	
	mu.Lock()
	_ = enc.Encode(kvCopy)
	mu.Unlock()
}