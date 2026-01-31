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

const (
	idBitSize      = 160
	idByteSize     = 20
	bitShiftBase   = 0x80
	bitsPerByte    = 8
	lastBitIndex   = 159
	candidateScale = 2
)

// xor computes XOR of two 20-byte arrays.
func xor(a, b [idByteSize]byte) [idByteSize]byte {
	var o [idByteSize]byte
	for i := range idByteSize {
		// nolint:gosec // G602 false positive: i is within [0, 19]
		o[i] = a[i] ^ b[i]
	}

	return o
}

// prefixLen finds the prefix length of bits from a 160-bit array.
func prefixLen(id [idByteSize]byte) int {
	for byteIndex := range idByteSize {
		// nolint:gosec // G602 false positive: byteIndex is within [0, 19]
		if id[byteIndex] == 0 {
			continue
		}

		for bitIndex := range bitsPerByte {
			// nolint:gosec // G602 false positive: byteIndex is within [0, 19]
			if id[byteIndex]&(bitShiftBase>>bitIndex) != 0 {
				return byteIndex*bitsPerByte + bitIndex
			}
		}
	}

	return lastBitIndex
}

const kSize = 8 // Bucket size

// Peer represents a DHT peer with ID, address, and last seen time.
type Peer struct {
	ID   [idByteSize]byte
	Addr *net.UDPAddr
	Time time.Time
}

type bucket struct{ peers []Peer }

// Table represents a DHT routing table.
type Table struct {
	mu     sync.RWMutex
	self   [idByteSize]byte
	bucket [idBitSize]bucket
}

// NewTable creates a new routing table with the given self ID.
func NewTable(self [idByteSize]byte) *Table {
	return &Table{self: self}
}

// Update inserts or refreshes peer p in the appropriate bucket.
// Self-ID is never stored.
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
	for _, buc := range t.bucket {
		for _, p := range buc.peers {
			peers = append(peers, p.Addr.String())
		}
	}

	logger.Log("RT peers update", map[string]any{
		"peers":           peers,
		"new_peer":        peer.Addr.String(),
		"new_peer_bucket": bucketIdx,
	})
}

// dist computes the XOR distance between two 20-byte IDs.
func dist(a, b [idByteSize]byte) *big.Int {
	xorResult := xor(a, b)

	return new(big.Int).SetBytes(xorResult[:])
}

// Closest finds the n closest peers to the target.
func (t *Table) Closest(target [idByteSize]byte, n int) []Peer {
	t.mu.RLock()
	defer t.mu.RUnlock()

	candidates := make([]Peer, 0, n*candidateScale)
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

// CheckAddresses returns all peer addresses in the routing table.
func (t *Table) CheckAddresses() []string {
	var nPeers int
	for _, b := range t.bucket {
		nPeers += len(b.peers)
	}

	addresses := make([]string, 0, nPeers)
	for _, b := range t.bucket {
		for _, p := range b.peers {
			addresses = append(addresses, p.Addr.String())
		}
	}

	return addresses
}

// GetNPeers returns up to n peers from the routing table.
func (t *Table) GetNPeers(n int) []*Peer {
	peers := make([]*Peer, 0, n)
	for _, b := range t.bucket {
		for _, p := range b.peers {
			if len(peers) >= n {
				return peers
			}

			peers = append(peers, &p)
		}
	}

	return peers
}
