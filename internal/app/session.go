// Owns the torrent's in-memory state

package app

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/peer"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
	"github.com/BitTorrentFileSharing/bittorrent/internal/util"
)

// Session owns the live state of a single .bit torrent
type Session struct {
	// Sync
	Mu sync.Mutex

	// Immutable Metadata
	Meta   *metainfo.Meta
	Pieces [][]byte         // len == number of pieces
	BF     storage.Bitfield // which pieces we own

	// subsystems
	DHT   *DHTService // nil when -dht-listen "" was passed
	Swarm *Swarm      // might start empty, peers added later

	// cfg reference (for subsystems)
	cfg *Config
}

// Allocates memory buffers, starts the UDP node
// and creates *empty* Swarm.
//
// It does not open any TCP connections or files yet.
func NewSession(cfg *Config, meta *metainfo.Meta) (*Session, error) {
	// In-memory buffers for pieces and bitfield
	var s *Session

	if meta == nil {
		s = &Session{
			cfg: cfg,
		}
	} else {
		s = &Session{
			Meta:   meta,
			Pieces: make([][]byte, len(meta.Hashes)),
			BF:     storage.NewBitfield(len(meta.Hashes)),
			cfg:    cfg,
		}
	}

	// UDP layer Boost
	dhtSvc, err := StartDHT(cfg.DHTListen, cfg.BootstrapCSV)
	if err != nil {
		return nil, err
	}
	s.DHT = dhtSvc

	// TCP layer Boost (runs only in Leecher side)
	// if cfg.MetaPath != "" {
	// 	s.Swarm = NewSwarm(s, cfg.DestDir, cfg.KeepSeedingSec)
	// }
	return s, nil
}

// Seeder path
func (s *Session) RunSeeder() error {
	cfg := s.cfg
	dataPath := cfg.SeedPath
	metaPath := dataPath + ".bit"

	// Load OR create .bit file
	if s.Meta == nil { // first run
		if err := s.ensureMeta(dataPath, metaPath); err != nil {
			return err
		}
	}

	// Load file pieces into RAM
	logger.Log("piece_cache_load", map[string]any{"file": dataPath})
	pieces, _, err := storage.Split(dataPath, storage.DefaultPiece)
	if err != nil {
		return err
	}
	for i, p := range pieces {
		// Seeder owns everything
		s.Pieces[i] = p
		s.BF.Set(i)
	}

	// UDP listener loop
	if s.DHT != nil {
		infoHash, _ := protocol.InfoHash(metaPath)
		maxTries := 5
		for range maxTries {
			var addresses []string = s.DHT.Node.RoutingTable.CheckAddresses()
			if addresses == nil {
				logger.Log("seeder did not find DHT yet... try again after 5 sec", nil)
				time.Sleep(5 * time.Second)
				continue
			}
			logger.Log("Seeder_announce", map[string]any{"dht": addresses})
			// This is very fucking important
			s.DHT.Announce(infoHash, cfg.Listen)
			break
		}
	}

	// TCP listener loop
	peerID := protocol.RandomPeerID()
	infoHash, _ := protocol.InfoHash(metaPath)
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return err
	}
	logger.Log(
		"seeder_ready",
		map[string]any{"file": dataPath, "tcp": cfg.Listen},
	)
	for {
		conn, err := ln.Accept()
		if err != nil {
			logger.Log(
				"accept_err",
				map[string]any{"err": err.Error()},
			)
			continue
		}
		// One goroutine per remote peer
		go func(c net.Conn) {
			p := newPeerAsSeeder(c, s.BF, peerID, s.Pieces, infoHash)
			logger.Log(
				"new_leecher",
				map[string]any{"peer": c.RemoteAddr().String()},
			)
			_ = p // peer goroutine handles traffic, nothing to do
		}(conn)
	}
}

// Creates .bit file when seeding for the first time.
// Also fills session meta-related fields if null
func (s *Session) ensureMeta(dataPath, metaPath string) error {
	if s.Meta != nil {
		return nil
	}
	if util.Exists(metaPath) {
		m, err := metainfo.Load(metaPath)
		if err != nil {
			return err
		}
		s.Meta = m
		s.Pieces = make([][]byte, len(m.Hashes))
		s.BF = storage.NewBitfield(len(m.Hashes))
		return nil
	}

	// Otherwise create a metafile
	pieces, hashes, err := storage.Split(dataPath, storage.DefaultPiece)
	if err != nil {
		return err
	}
	meta := &metainfo.Meta{
		FileName:   filepath.Base(dataPath),
		FileLength: int64(len(pieces) * storage.DefaultPiece),
		PieceSize:  storage.DefaultPiece,
		Hashes:     hashes,
	}
	if err := meta.Write(metaPath); err != nil {
		return err
	}
	logger.Log("meta_write", map[string]any{"file": metaPath})
	s.Meta = meta

	// Also Update other fields
	s.Pieces = make([][]byte, len(meta.Hashes))
	s.BF = storage.NewBitfield(len(meta.Hashes))

	return nil
}

// Helper to wrap peer.New with seeder-specific fields.
func newPeerAsSeeder(c net.Conn, bf storage.Bitfield, id [20]byte,
	allPieces [][]byte, infoHash [20]byte) *peer.Peer {

	p := peer.New(c, bf, id) // Spawn threads btw
	p.Pieces = allPieces
	p.SendCh <- protocol.NewHandshake(infoHash[:], id[:])
	p.SendCh <- protocol.NewBitfield(bf)
	return p
}

//
// Leecher path
//

// Runs leecher. Will seed after getting a file if specified.
func (s *Session) RunLeecher() error {
	cfg := s.cfg

	// Load .bit
	meta, err := metainfo.Load(cfg.MetaPath)
	if err != nil {
		logger.Log(
			"leecher_load_metainfo_err",
			map[string]any{"error": err.Error()},
		)
		return err
	}

	// Update session fields
	s.Meta = meta
	s.Pieces = make([][]byte, len(meta.Hashes))
	s.BF = storage.NewBitfield(len(meta.Hashes))

	// First goal - find seeders
	if s.DHT == nil {
		return errors.New("specify dht")
	}

	infoHash, _ := protocol.InfoHash(cfg.MetaPath)
	// Try 100 times to find seeder
	maxTries := 100
	for range maxTries {
		peers := s.DHT.LookupPeers(infoHash)
		if len(peers) == 0 {
			time.Sleep(5 * time.Second)
			continue
		}
		if len(peers) > 0 {
			cfg.PeersCSV += "," + strings.Join(peers, ",")
			logger.Log("leecher_bootstrap", map[string]any{"new_peers": peers})
			break
		}
	}

	// TCP side
	s.Swarm = NewSwarm(s, cfg.DestDir, cfg.KeepSeedingSec)
	s.Swarm.Dial(cfg.PeersCSV)
	s.Swarm.Loop() // Blocks until the file is complete

	// Starts seeding
	// Code is similar to runSeeder there
	if cfg.KeepSeedingSec > 0 {
		if cfg.Listen == ":0" {
			return errors.New("please, specify exact tcp address in order to seed '-tcp-listen x'")
		}
		errCh := make(chan error, 1)

		go func() {
			defer func() {
				if r := recover(); r != nil {
					errCh <- fmt.Errorf("seeder panic: %v", r)
				}
			}()

			logger.Log("seeder_ready", map[string]any{
				"file": strings.TrimSuffix(filepath.Base(cfg.MetaPath), ".bit"),
				"tcp":  cfg.Listen})

			// 1. Announce itself for known peers
			s.DHT.Announce(infoHash, cfg.Listen)

			// 2. Open TCP listener and serve incoming messages
			ln, err := net.Listen("tcp", cfg.Listen)
			if err != nil {
				errCh <- fmt.Errorf("failed to listen on %s: %w", cfg.Listen, err)
				return
			}

			// close listener after n sec
			go func() {
				<-time.After(time.Duration(cfg.KeepSeedingSec) * time.Second)
				ln.Close()
			}()

			for {
				conn, err := ln.Accept()
				if err != nil {
					// Error via closing due time?
					if opErr, ok := err.(*net.OpError); ok && opErr.Err.Error() == "use of closed network connection" {
						break
					}
					logger.Log(
						"accept_err",
						map[string]any{"err": err.Error()},
					)
					continue
				}

				// One routine per remote connection
				go func(c net.Conn) {
					newPeerAsSeeder(c, s.BF, protocol.RandomPeerID(), s.Pieces, infoHash)
					logger.Log(
						"new_leecher",
						map[string]any{"peer": c.RemoteAddr().String()},
					)
				}(conn)
			}

			<-time.After(time.Duration(cfg.KeepSeedingSec) * time.Second)
			logger.Log("leecher_stopped_seeding", nil)
			errCh <- nil
		}()

		if err := <-errCh; err != nil {
			return err
		}
	}
	return nil
}

// Saves data & sets bit
func (s *Session) MarkPiece(idx int, data []byte) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Pieces[idx] = data
	s.BF.Set(idx)
}
