// Package app owns the torrent's in-memory state and orchestrates downloads.
package app

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BitTorrentFileSharing/bittorrent/internal/bitutil"
	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/peer"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

const (
	dhtAnnounceMaxTries  = 5
	leecherLookupRetries = 100
)

// Session owns the live state of a single .bit torrent.
type Session struct {
	// Sync
	Mu sync.Mutex

	// Immutable Metadata
	InfoHash [20]byte
	Meta     *metainfo.Meta
	Pieces   [][]byte         // len == number of pieces
	BF       storage.Bitfield // which pieces we own

	// subsystems
	DHT   *DHTService // nil when -dht-listen "" was passed
	Swarm *Swarm      // might start empty, peers added later

	// cfg reference (for subsystems)
	cfg *Config
}

// NewSession allocates memory buffers and starts the UDP node.
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
	if err != nil && !errors.Is(err, ErrDHTDisabled) {
		return nil, err
	}

	s.DHT = dhtSvc

	return s, nil
}

// RunSeeder runs the seeder path.
func (sess *Session) RunSeeder() error {
	cfg := sess.cfg
	dataPath := cfg.SeedPath
	metaPath := dataPath + ".bit"

	// Load OR create .bit file
	if sess.Meta == nil { // first run
		if err := sess.ensureMeta(dataPath, metaPath); err != nil {
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
		sess.Pieces[i] = p
		sess.BF.Set(i)
	}

	// UDP listener loop
	if sess.DHT != nil {
		infoHash, _ := protocol.InfoHash(metaPath)
		go sess.announceInLoop(infoHash)
	}

	// TCP listener loop
	peerID := protocol.RandomPeerID()
	infoHash, _ := protocol.InfoHash(metaPath)
	sess.InfoHash = infoHash

	lc := net.ListenConfig{}
	ln, err := lc.Listen(context.Background(), "tcp", cfg.Listen)
	if err != nil {
		return err
	}

	logger.Log(
		"seeder_ready",
		map[string]any{
			"file":     dataPath,
			"tcp":      cfg.Listen,
			"infoHash": hex.EncodeToString(infoHash[:]),
		},
	)

	sess.serveTCP(ln, infoHash, peerID)

	return nil
}

func (sess *Session) announceInLoop(infoHash [20]byte) {
	for range dhtAnnounceMaxTries {
		addresses := sess.DHT.Node.RoutingTable.CheckAddresses()
		if addresses == nil {
			logger.Log("seeder did not find DHT yet... try again after 5 sec", nil)
			time.Sleep(dhtAnnounceRetryDelay)

			continue
		}

		logger.Log("Seeder_announce", map[string]any{"dht": addresses})
		sess.DHT.Announce(infoHash, sess.cfg.Listen)

		break
	}
}

func (sess *Session) serveTCP(ln net.Listener, infoHash [20]byte, peerID [20]byte) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				break
			}

			logger.Log("accept_err", map[string]any{"err": err.Error()})

			continue
		}

		// One goroutine per remote peer
		go func(c net.Conn) {
			p := newPeerAsSeeder(c, sess.BF, peerID, sess.Pieces, infoHash)
			logger.Log("new_leecher", map[string]any{"peer": c.RemoteAddr().String()})
			_ = p
		}(conn)
	}
}

// ensureMeta creates .bit file when seeding for the first time.
// Also fills session meta-related fields if null.
func (sess *Session) ensureMeta(dataPath, metaPath string) error {
	if sess.Meta != nil {
		return nil
	}

	if bitutil.Exists(metaPath) {
		m, err := metainfo.Load(metaPath)
		if err != nil {
			return err
		}

		sess.Meta = m
		sess.Pieces = make([][]byte, len(m.Hashes))
		sess.BF = storage.NewBitfield(len(m.Hashes))

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
	sess.Meta = meta

	// Also Update other fields
	sess.Pieces = make([][]byte, len(meta.Hashes))
	sess.BF = storage.NewBitfield(len(meta.Hashes))

	return nil
}

// newPeerAsSeeder is a helper to wrap peer.New with seeder-specific fields.
func newPeerAsSeeder(c net.Conn, bf storage.Bitfield, id [20]byte,
	allPieces [][]byte, infoHash [20]byte,
) *peer.Peer {
	p := peer.New(c, bf, id, infoHash) // Spawn threads btw
	p.Pieces = allPieces
	logger.Log("send_handshake", map[string]any{
		"infoHash": hex.EncodeToString(infoHash[:]),
	})
	p.SendCh <- protocol.NewHandshake(infoHash[:], id[:])
	p.SendCh <- protocol.NewBitfield(bf)

	return p
}

// RunLeecher runs the leecher. Will seed after getting a file if specified.
func (sess *Session) RunLeecher() error {
	cfg := sess.cfg

	// Load .bit
	meta, err := metainfo.Load(cfg.MetaPath)
	if err != nil {
		logger.Log("leecher_load_metainfo_err", map[string]any{"error": err.Error()})

		return err
	}

	// Update session fields
	sess.Meta = meta
	sess.Pieces = make([][]byte, len(meta.Hashes))
	sess.BF = storage.NewBitfield(len(meta.Hashes))

	// First goal - find seeders
	if sess.DHT == nil {
		return errors.New("specify dht")
	}

	infoHash, _ := protocol.InfoHash(cfg.MetaPath)
	sess.InfoHash = infoHash
	logger.Log("leecher", map[string]any{
		"desired_infoHash": hex.EncodeToString(infoHash[:]),
	})

	if err := sess.findSeeders(infoHash); err != nil {
		return err
	}

	// TCP side
	sess.Swarm = NewSwarm(sess, cfg.DestDir, cfg.KeepSeedingSec)
	sess.Swarm.Dial(cfg.PeersCSV, infoHash)
	sess.Swarm.Loop() // Blocks until the file is complete

	// Starts seeding
	if cfg.KeepSeedingSec > 0 {
		return sess.keepSeeding(infoHash)
	}

	return nil
}

func (sess *Session) findSeeders(infoHash [20]byte) error {
	for range leecherLookupRetries {
		peers := sess.DHT.LookupPeers(infoHash)
		if len(peers) == 0 {
			time.Sleep(dhtAnnounceRetryDelay)

			continue
		}

		sess.cfg.PeersCSV += "," + strings.Join(peers, ",")
		logger.Log("leecher_bootstrap", map[string]any{"new_peers": peers})

		return nil
	}

	return nil
}

func (sess *Session) keepSeeding(infoHash [20]byte) error {
	cfg := sess.cfg
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
			"tcp":  cfg.Listen,
		})

		sess.DHT.Announce(infoHash, cfg.Listen)

		lc := net.ListenConfig{}
		ln, err := lc.Listen(context.Background(), "tcp", cfg.Listen)
		if err != nil {
			errCh <- fmt.Errorf("failed to listen on %s: %w", cfg.Listen, err)

			return
		}

		go func() {
			<-time.After(time.Duration(cfg.KeepSeedingSec) * time.Second)
			_ = ln.Close()
		}()

		sess.serveTCP(ln, infoHash, protocol.RandomPeerID())

		logger.Log("leecher_stopped_seeding", nil)
		errCh <- nil
	}()

	return <-errCh
}

// MarkPiece saves data and sets the bit for a piece.
func (sess *Session) MarkPiece(idx int, data []byte) {
	sess.Mu.Lock()
	defer sess.Mu.Unlock()

	sess.Pieces[idx] = data
	sess.BF.Set(idx)
}
