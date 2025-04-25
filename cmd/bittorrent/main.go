package main

import (
	"os"

	"github.com/BitTorrentFileSharing/bittorrent/internal/app"
	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
)

func main() {
	cfg := app.ParseFlags()

	// Create a "blank" session (no meta that depends on the mode)
	sess, err := app.NewSession(cfg, nil)
	if err != nil {
		logger.Log("fatal", map[string]any{"err": err.Error()})
		os.Exit(1)
	}

	// Decide file-sharing role
	switch {
	case cfg.SeedPath != "":
		logger.Log("role", map[string]any{"mode": "seeder"})
		err = sess.RunSeeder()
	case cfg.MetaPath != "":
		logger.Log("role", map[string]any{"mode": "leecher"})
		err = sess.RunLeecher()
	default:
		// Pure DHT bootstrap: session already started UDP node
		// so it is just blocking
		logger.Log("role", map[string]any{"mode": "dht-node"})
		select {}
	}
	if err != nil {
		logger.Log("fatal", map[string]any{"err": err.Error()})
		os.Exit(1)
	}
}
