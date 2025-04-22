package app

import (
	"log"
	"net"
	"path/filepath"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/peer"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
	"github.com/BitTorrentFileSharing/bittorrent/internal/util"
)

func RunSeeder(cfg *Config) error {
	data := cfg.SeedPath
	metaPath := data + ".bit"

	// 1. Load or Create metainfo
	var meta *metainfo.Meta
	if util.Exists(metaPath) {
		m, err := metainfo.Load(metaPath)
		if err != nil {
			return err
		}
		meta = m
	} else {
		pieces, hashes, err := storage.Split(data, storage.DefaultPiece)
		if err != nil {
			return err
		}
		meta = &metainfo.Meta{
			FileName:   filepath.Base(data),
			FileLength: int64(len(pieces) * storage.DefaultPiece),
			PieceSize:  storage.DefaultPiece,
			Hashes:     hashes,
		}
		if err := meta.Write(metaPath); err != nil {
			return err
		}
		logger.Log("meta_write", map[string]any{"file": metaPath})
	}

	// Always need cache pieces in RAM for uploads
	pieces, _, err := storage.Split(data, storage.DefaultPiece)
	if err != nil {
		return err
	}

	// 2. Define peer and connection
	bf := storage.NewBitfield(len(meta.Hashes))
	for i := range bf {
		bf.Set(i) // Seeder owns anything
	}
	peerID := protocol.RandomPeerID()
	infoHash, _ := protocol.InfoHash(metaPath)

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	log.Println("[seeder] listening on", cfg.Listen)

	// Listening loop
	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("[!] accept:", err)
			continue
		}

		// For each connection seeder creates a new peer
		p := peer.New(conn, bf, peerID)
		p.Pieces = pieces

		p.SendCh <- protocol.NewHandshake(infoHash[:], peerID[:])
		p.SendCh <- protocol.NewBitfield(bf)

		logger.Log("upload_conn", map[string]any{"peer": conn.RemoteAddr().String()})
	}
}
