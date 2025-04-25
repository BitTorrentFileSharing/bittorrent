package peer

import (
	"crypto/sha1"
	"encoding/binary"
	"io"
	"log"
	"net"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
	"github.com/BitTorrentFileSharing/bittorrent/internal/util"
)

type Peer struct {
	Conn     net.Conn
	Bitfield storage.Bitfield
	SendCh   chan protocol.Message
	Pieces   [][]byte // Download buffer (leecher)
	Meta     *metainfo.Meta
	ID       [20]byte
	OnHave   func(int) // Callback into piece picker
}

func New(conn net.Conn, bf storage.Bitfield, id [20]byte) *Peer {
	peer := &Peer{Conn: conn, Bitfield: bf, SendCh: make(chan protocol.Message, 16), ID: id}
	go peer.writer()
	go peer.reader()
	return peer
}

// Writes messages into connection
func (peer *Peer) writer() {
	for msg := range peer.SendCh {
		if err := msg.Encode(peer.Conn); err != nil {
			log.Println("Got writer error:", err)
			return
		}
	}
}

// Reads messages from connection
func (peer *Peer) reader() {
	// Leaving callback
	defer func() {
		if peer.OnHave != nil {
			peer.OnHave(-1)
		}
	}()

	for {
		msg, err := protocol.Decode(peer.Conn)
		if err == io.EOF {
			logger.Log(
				"bye_leecher",
				map[string]any{"bye": peer.Conn.RemoteAddr().String()},
			)
			return
		} else if err != nil {
			logger.Log(
				"decode_err",
				map[string]any{"err": err.Error()},
			)
			return
		}
		// log.Println("[+] Received message, len:", len(msg.Data))
		peer.handle(msg)
	}
}

// TODO: Write doc for each case
func (peer *Peer) handle(message *protocol.Message) {
	switch message.ID {
	case protocol.MsgHandshake:
		// DATA IS:
		// 20 bytes infoHash
		// 20 bytes peerID

	case protocol.MsgBitfield:
		// DATA IS:
		// n bytes (bitfield)
		peer.Bitfield = storage.ParseBitfield(message.Data)

	case protocol.MsgRequest:
		// DATA IS:
		// 4 bytes piece index
		// 4 bytes offset
		// 4 bytes length

		// I ignore offset/len and always send full piece now
		// Sorry

		if peer.Pieces == nil {
			// Leecher ignores
			return
		}
		idx := int(binary.BigEndian.Uint32(message.Data)) // 4-byte index
		piece := peer.Pieces[idx]
		resp := protocol.NewPiece(idx, piece)
		peer.SendCh <- resp

	case protocol.MsgHave:
		idx := int(binary.BigEndian.Uint32(message.Data))
		peer.Bitfield.Set(idx)
		if peer.OnHave != nil {
			peer.OnHave(idx)
		}

	// Piece came.
	// Verifies hash and Notifies other peers
	case protocol.MsgPiece:
		// DATA IS:
		// 4 bytes piece index
		// 4 bytes offset
		// n bytes actual data
		idx := int(binary.BigEndian.Uint32(message.Data[:4]))
		data := message.Data[8:] // We skip index+offset

		// Verify Hash
		if sha1.Sum(data) != util.Sha1Sum(peer.Meta.Hashes[idx]) {
			log.Printf("Bad hash for piece %d\n", idx)
			return
		}

		peer.Pieces[idx] = data
		peer.Bitfield.Set(idx)

		// 1. Notify uploader immediately
		haveMsg := protocol.Message{
			ID:   protocol.MsgHave,
			Data: util.Uint32ToBytes(uint32(idx)),
		}
		// Goes to uploader via same conn
		peer.SendCh <- haveMsg

		// 2. Then inform piece-picker layer
		if peer.OnHave != nil {
			peer.OnHave(idx) // Upper-layer will fan this out to others
		}

	}
}
