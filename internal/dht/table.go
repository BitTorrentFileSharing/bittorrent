package dht

import (
	"math/big"
	"net"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
)

// / Some Helpers below
func xor(a, b [20]byte) (o [20]byte) {
	for i := range 20 {
		o[i] = a[i] ^ b[i]
	}
	return
}

// Finds the prefix len of bits from 160bits array
// Let's pretend it is working
func prefixLen(id [20]byte) int {
	// Traverse each byte
	for byteIndex := range 20 {
		// 8 bits are zero -> skip
		if id[byteIndex] == 0 {
			continue
		}
		// This byte is not 0 - check bits
		for bitIndex := range 8 {
			if id[byteIndex]&(0x80>>bitIndex) != 0 {
				return byteIndex*8 + bitIndex
			}
		}
	}
	return 159
}

const kSize = 8 // Bucket size

type Peer struct {
	ID   [20]byte
	Addr *net.UDPAddr
	Time time.Time
}

type bucket struct{ peers []Peer }

type Table struct {
	mu     sync.RWMutex
	self   [20]byte
	bucket [160]bucket
}

func NewTable(self [20]byte) *Table {
	return &Table{self: self}
}

// Inserts or Refreshes peer *p* in the appropriate bucket.
//   - Self-ID is never stored.
func (t *Table) Update(peer Peer) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// 1. Ignore ourselves
	if peer.ID == t.self {
		return
	}
	
	bucketIdx := prefixLen(xor(peer.ID, t.self))
	b := &t.bucket[bucketIdx]

	// 2. Remove existing instance (refresh)
	for idx, bucketPeer := range b.peers {
		if bucketPeer.ID == peer.ID {
			b.peers = slices.Delete(b.peers, idx, idx+1)
			break
		}
	}

	// 3. Append as most-recent
	peer.Time = time.Now()
	b.peers = append(b.peers, peer)

	// 4. Evict LRU if we now exceed K
	if len(b.peers) > kSize {
		b.peers = b.peers[1:]
	}

	// FOR LOGS: Check current peers
	var peers []string
	for _, bucket := range t.bucket {
		for _, peer := range bucket.peers {
			peers = append(peers, peer.Addr.String())
		}
	}

	logger.Log("RT peers update", map[string]any{"peers": peers, "new_peer": peer.Addr.String(), "new_peer_bucket": bucketIdx})
}

/// Query closest

func dist(a, b [20]byte) *big.Int {
	xorResult := xor(a, b)
	return new(big.Int).SetBytes(xorResult[:])
}

// Finds closest peer to the target
func (t *Table) Closest(target [20]byte, n int) []Peer {
	t.mu.RLock()
	defer t.mu.RUnlock()
	
	candidates := make([]Peer, 0, n*2)
	for _, b := range t.bucket {
		candidates = append(candidates, b.peers...)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return dist(candidates[i].ID, target).Cmp(dist(candidates[j].ID, target)) < 0
	})
	// Shorten
	if len(candidates) > n {
		candidates = candidates[:n]
	}
	return candidates
}

func (table *Table) CheckAddresses() []string {
	var addresses []string
	for _, bucket := range table.bucket {
		for _, peer := range bucket.peers {
			addresses = append(addresses, peer.Addr.String())
		}
	}

	return addresses
}

func (table *Table) GetNPeers(n int) []*Peer {
	var peers []*Peer
	for _, bucket := range table.bucket {
		for _, peer := range bucket.peers {
			if len(peers) >= n {
				return peers
			}
			peers = append(peers, &peer)
		}
	}

	return peers
}
