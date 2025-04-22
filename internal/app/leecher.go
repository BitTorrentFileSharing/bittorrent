package app

import (
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
)

func RunLeecher(cfg *Config) error {
	// 0. Load meta data
	meta, err := metainfo.Load(cfg.MetaPath)
	if err != nil {
		return err
	}

	// 1. Create session & picker
	session := NewSession(meta)
	swarm := NewSwarm(session, cfg.DestDir, cfg.KeepSeedingSec)
	swarm.Dial(cfg.PeersCSV)

	swarm.Loop()
	return nil
}
