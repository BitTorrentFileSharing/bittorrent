package peer

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
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
	Conn            net.Conn
	Bitfield        storage.Bitfield
	SendCh          chan protocol.Message
	Meta            *metainfo.Meta
	Pieces          [][]byte  // Download buffer for leecher
	ID              [20]byte  // Our ID
	RemoteID        [20]byte  // Remote ID
	OnHave          func(int) // Callback into piece picker
	desiredInfohash [20]byte
	handshakeDone   bool
}

func New(conn net.Conn, bf storage.Bitfield, id, desiredInfohash [20]byte) *Peer {
	peer := &Peer{Conn: conn, Bitfield: bf, SendCh: make(chan protocol.Message, 16), ID: id, desiredInfohash: desiredInfohash}
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
		peer.handle(msg)
	}
}

func (peer *Peer) handle(message *protocol.Message) {
    if !peer.handshakeDone && message.ID != protocol.MsgHandshake {
        logger.Log("unexpected_message_before_handshake", map[string]any{
            "peer": peer.Conn.RemoteAddr().String(),
            "messageID": message.ID,
        })
        return
    }

	switch message.ID {
	case protocol.MsgHandshake:
		if len(message.Data) != 40 {
			logger.Log("bad_handshake",
				map[string]any{"peer": peer.Conn.RemoteAddr().String(), "reason": "len"})
			return
		}
		infoHash := message.Data[:20]
		copy(peer.RemoteID[:], message.Data[20:40])

		logger.Log("recv_handshake", map[string]any{
			"infoHash": hex.EncodeToString(infoHash[:]),
			"expected": hex.EncodeToString(peer.desiredInfohash[:]),
		})

		if !bytes.Equal(infoHash, peer.desiredInfohash[:]) {
			logger.Log("infohash_mismatch", nil)
			peer.Conn.Close()
			return
		}

		peer.handshakeDone = true
		logger.Log("handshake_ok",
			map[string]any{"peer": peer.Conn.RemoteAddr().String()})
		return

	case protocol.MsgBitfield:
		peer.Bitfield = storage.ParseBitfield(message.Data)

	case protocol.MsgRequest:
		// I ignore offset/len and always send full piece now

		if peer.Pieces == nil {
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

	default:
		logger.Log("unknown_message_id", map[string]any{
            "peer": peer.Conn.RemoteAddr().String(),
            "messageID": message.ID,
        })
	}
}
