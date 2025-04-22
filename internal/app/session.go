// Owns the torrent's in-memory state

package app

import (
	"sync"

	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

type Session struct {
	Meta   *metainfo.Meta
	Pieces [][]byte
	BF     storage.Bitfield
	Mu     sync.Mutex
}

// Creates session with meta, empty pieces and bitfield.
func NewSession(meta *metainfo.Meta) *Session {
	return &Session{
		Meta:   meta,
		Pieces: make([][]byte, len(meta.Hashes)),
		BF:     storage.NewBitfield(len(meta.Hashes)),
	}
}

// Saves data & sets bit
func (s *Session) MarkPiece (idx int, data []byte) {
	s.Mu.Lock()
	defer s.Mu.Unlock()
	s.Pieces[idx] = data
	s.BF.Set(idx)
}
