package app

import (
	"math/rand"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/peer"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

// Swarm manages peers
type Swarm struct {
	Sess  *Session // Shared pieces & bitfield
	Peers []*peer.Peer
	mu    sync.Mutex

	// state for rarest-first
	missing      []bool
	availability []int
	ticker       *time.Ticker

	// Specific for each leecher
	destDir string
	keepSec int

	isDone chan bool
}

const tickerPeriod = time.Duration(2 * time.Second)

// Creates new swarm taking session
func NewSwarm(sess *Session, destDir string, keep int) *Swarm {
	n := len(sess.Meta.Hashes)
	miss := make([]bool, n)
	for i := range miss {
		miss[i] = true
	}
	return &Swarm{
		Sess:         sess,
		mu:           sync.Mutex{},
		missing:      miss,
		availability: make([]int, n),
		isDone:       make(chan bool, 1),
		ticker:       time.NewTicker(tickerPeriod),
		destDir:      destDir,
		keepSec:      keep,
	}
}

// Dial CSV peers and attach to Swarm
func (sw *Swarm) Dial(csv string) {
	infoHash, _ := protocol.InfoHash(sw.Sess.Meta.FileName + ".bit")
	for addr := range strings.SplitSeq(csv, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}
		go func(a string) {
			// Join peer to network
			conn, err := net.Dial("tcp", a)
			if err != nil {
				logger.Log(
					"dial_err",
					map[string]any{"peer": a, "err": err.Error()},
				)
				return
			}
			logger.Log("joined_to_peer", map[string]any{"peer": a})

			p := peer.New(conn, sw.Sess.BF, protocol.RandomPeerID())
			p.Meta = sw.Sess.Meta
			p.Pieces = sw.Sess.Pieces

			p.OnHave = func(idx int) { sw.onHave(p, idx) }

			p.SendCh <- protocol.NewHandshake(infoHash[:], p.ID[:])
			p.SendCh <- protocol.NewBitfield(sw.Sess.BF)

			sw.mu.Lock()
			sw.Peers = append(sw.Peers, p)
			sw.mu.Unlock()
		}(addr)
	}
}

// Central callback when *any* peer finishes a piece or disconnects
func (sw *Swarm) onHave(src *peer.Peer, idx int) {
	if idx == -1 { // Disconnect
		logger.Log("leave", map[string]any{"peer": src.Conn.RemoteAddr().String()})
		return
	}
	if !sw.missing[idx] { // Got a piece that was owned already
		return
	}

	// Mark done
	sw.missing[idx] = false
	sw.Sess.BF.Set(idx)

	// Check if load is complete
	done := 0
	for _, need := range sw.missing {
		if !need {
			done++
		}
	}

	logger.Log("have", map[string]any{"piece": idx, "totalPieces": done})

	// Broadcast to everyone else
	have := protocol.NewHave(idx)
	sw.mu.Lock()
	for _, p := range sw.Peers {
		if p != src {
			p.SendCh <- have
		}
	}
	sw.mu.Unlock()

	// Finished taking algorithm
	if done == len(sw.missing) {
		outPath := filepath.Join(sw.destDir, sw.Sess.Meta.FileName)
		if err := storage.Join(sw.Sess.Pieces, outPath); err != nil {
			logger.Log("write_err", map[string]any{"err": err.Error()})
		} else {
			logger.Log("complete", map[string]any{"file": outPath})
		}
		sw.isDone <- true
	}

	// Ask for a new piece
	next := sw.choosePiece()
	if next != -1 {
		sw.request(next)
	}
}

// Start rarest-first loop (blocking)
func (sw *Swarm) Loop() {
	for {
		select {
		case <-sw.ticker.C:
			idx := sw.choosePiece()
			if idx == -1 {
				continue
			}
			sw.request(idx)
		case <-sw.isDone:
			sw.ticker.Stop()
			return
		}
	}
}

// Return rarest piece index by computing availability
func (sw *Swarm) choosePiece() int {
	for i := range sw.availability {
		sw.availability[i] = 0
	}
	sw.mu.Lock()
	// Here available peers bitfield compared
	for _, p := range sw.Peers {
		for i := range sw.availability {
			if p.Bitfield.Has(i) {
				sw.availability[i]++
			}
		}
	}
	sw.mu.Unlock()

	best := -1
	for i, need := range sw.missing {
		if !need { // No need to ask available piece
			continue
		}
		if best == -1 || sw.availability[i] < sw.availability[best] {
			best = i
		}
	}
	return best
}

// Ask random peer to send piece indexed *idx*
func (sw *Swarm) request(idx int) {
	sw.mu.Lock()
	defer sw.mu.Unlock()
	// For fun I will ask a random peer, not a first one
	var goodPeers []*peer.Peer
	for _, p := range sw.Peers {
		if p.Bitfield.Has(idx) {
			goodPeers = append(goodPeers, p)
		}
	}
	if len(goodPeers) == 0 {
		return // No available peers
	}
	randomPeerIdx := rand.Intn(len(goodPeers))
	chosenPeer := goodPeers[randomPeerIdx]
	
	// Send request
	chosenPeer.SendCh <- protocol.NewRequest(idx)
	logger.Log(
		"request",
		map[string]any{"piece": idx, "peer": chosenPeer.Conn.RemoteAddr().String()},
	)
}
