package peer

import (
	"crypto/sha1"
	"encoding/binary"
	"io"
	"log"
	"net"

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

func (peer *Peer) writer() {
	for msg := range peer.SendCh {
		if err := msg.Encode(peer.Conn); err != nil {
			log.Println("Got writer error:", err)
			return
		}
	}
}

func (peer *Peer) reader() {
	for {
		msg, err := protocol.Decode(peer.Conn)
		if err == io.EOF {
			log.Println("[*] peer closed connection")
			return
		} else if err != nil {
			log.Println("[!] decode error in reader:", err)
			return
		}
		log.Println("[+] Received message, len:", len(msg.Data))
		peer.handle(msg)
	}
}

// Handles 3 message types
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
		idx := int(binary.BigEndian.Uint32(message.Data[:4])) // 4-byte index
		piece := peer.Pieces[idx]
		resp := protocol.Message{
			ID: protocol.MsgPiece,
			// Might change data later
			Data: append(
				append(
					util.Uint32ToBytes(uint32(idx)),
					util.Uint32ToBytes(0)...),
				piece...),
		}
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
		// Create MsgHave
		have := protocol.Message{
			ID:   protocol.MsgHave,
			Data: util.Uint32ToBytes(uint32(idx)),
		}
		// Send it to other peers
		peer.SendCh <- have
		// Run Callback
		if peer.OnHave != nil {
			peer.OnHave(idx)
		}
	}
}
