// Package peer handles peer connections and protocol message handling.
package peer

import (
	"bytes"
	"crypto/sha1" // nolint:gosec // SHA-1 is required by the BitTorrent protocol
	"encoding/binary"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"

	"github.com/BitTorrentFileSharing/bittorrent/internal/bitutil"
	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

const (
	peerSendChSize   = 16
	handshakeDataLen = 40
	idSize           = 20
	pieceOffset      = 8
)

// Peer represents a connection to a remote peer.
type Peer struct {
	Conn            net.Conn
	Bitfield        storage.Bitfield
	SendCh          chan protocol.Message
	Meta            *metainfo.Meta
	Pieces          [][]byte     // Download buffer for leecher
	ID              [idSize]byte // Our ID
	RemoteID        [idSize]byte // Remote ID
	OnHave          func(int)    // Callback into piece picker
	desiredInfohash [idSize]byte
	handshakeDone   bool
}

// New creates a new peer connection and starts writer/reader goroutines.
func New(conn net.Conn, bf storage.Bitfield, id, desiredInfohash [idSize]byte) *Peer {
	peer := &Peer{
		Conn:            conn,
		Bitfield:        bf,
		SendCh:          make(chan protocol.Message, peerSendChSize),
		ID:              id,
		desiredInfohash: desiredInfohash,
	}
	go peer.writer()
	go peer.reader()

	return peer
}

// writer writes messages into the connection.
func (peer *Peer) writer() {
	for msg := range peer.SendCh {
		if err := msg.Encode(peer.Conn); err != nil {
			log.Println("Got writer error:", err)

			return
		}
	}
}

// reader reads messages from the connection.
func (peer *Peer) reader() {
	// Leaving callback
	defer func() {
		if peer.OnHave != nil {
			peer.OnHave(-1)
		}
	}()

	for {
		msg, err := protocol.Decode(peer.Conn)
		if errors.Is(err, io.EOF) {
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
			"peer":      peer.Conn.RemoteAddr().String(),
			"messageID": message.ID,
		})

		return
	}

	switch message.ID {
	case protocol.MsgHandshake:
		peer.handleHandshake(message)
	case protocol.MsgBitfield:
		peer.handleBitfield(message)
	case protocol.MsgRequest:
		peer.handleRequest(message)
	case protocol.MsgHave:
		peer.handleHave(message)
	case protocol.MsgPiece:
		peer.handlePiece(message)
	default:
		logger.Log("unknown_message_id", map[string]any{
			"peer":      peer.Conn.RemoteAddr().String(),
			"messageID": message.ID,
		})
	}
}

func (peer *Peer) handleHandshake(message *protocol.Message) {
	if len(message.Data) != handshakeDataLen {
		logger.Log("bad_handshake",
			map[string]any{"peer": peer.Conn.RemoteAddr().String(), "reason": "len"})

		return
	}

	infoHash := message.Data[:idSize]
	copy(peer.RemoteID[:], message.Data[idSize:handshakeDataLen])

	logger.Log("recv_handshake", map[string]any{
		"infoHash": hex.EncodeToString(infoHash),
		"expected": hex.EncodeToString(peer.desiredInfohash[:]),
	})

	if !bytes.Equal(infoHash, peer.desiredInfohash[:]) {
		logger.Log("infohash_mismatch", nil)
		_ = peer.Conn.Close()

		return
	}

	peer.handshakeDone = true
	logger.Log("handshake_ok",
		map[string]any{"peer": peer.Conn.RemoteAddr().String()})
}

func (peer *Peer) handleBitfield(message *protocol.Message) {
	peer.Bitfield = storage.ParseBitfield(message.Data)
}

func (peer *Peer) handleRequest(message *protocol.Message) {
	if peer.Pieces == nil {
		return
	}

	idx := int(binary.BigEndian.Uint32(message.Data)) // 4-byte index
	piece := peer.Pieces[idx]
	resp := protocol.NewPiece(idx, piece)
	peer.SendCh <- resp
}

func (peer *Peer) handleHave(message *protocol.Message) {
	idx := int(binary.BigEndian.Uint32(message.Data))
	peer.Bitfield.Set(idx)

	if peer.OnHave != nil {
		peer.OnHave(idx)
	}
}

func (peer *Peer) handlePiece(message *protocol.Message) {
	idx := int(binary.BigEndian.Uint32(message.Data[:4]))
	data := message.Data[pieceOffset:] // We skip index+offset

	// Verify Hash
	// nolint:gosec // SHA-1 is required by the BitTorrent protocol
	if sha1.Sum(data) != bitutil.Sha1Sum(peer.Meta.Hashes[idx]) {
		log.Printf("Bad hash for piece %d\n", idx)

		return
	}

	peer.Pieces[idx] = data
	peer.Bitfield.Set(idx)

	// 1. Notify uploader immediately
	haveMsg := protocol.Message{
		ID: protocol.MsgHave,
		// nolint:gosec // idx is within uint32 range for BitTorrent
		Data: bitutil.Uint32ToBytes(uint32(idx)),
	}
	// Goes to uploader via same conn
	peer.SendCh <- haveMsg

	// 2. Then inform piece-picker layer
	if peer.OnHave != nil {
		peer.OnHave(idx) // Upper-layer will fan this out to others
	}
}
