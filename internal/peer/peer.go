package peer

import (
	"encoding/binary"
	"log"
	"net"
	"io"

	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
)

type Peer struct {
	Conn     net.Conn
	Bitfield storage.Bitfield
	SendCh   chan protocol.Message
	Pieces   [][]byte // Only set on seeder
	ID       [20]byte
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
		log.Println("[*] Received message with data length:", len(msg.Data))
		peer.handle(msg)
	}
}

// Handles 3 message types
func (peer *Peer) handle(m *protocol.Message) {
	switch m.ID {
	case protocol.MsgHandshake:
		// Nothing to do. Already exchanged

	case protocol.MsgBitfield:
		peer.Bitfield = storage.ParseBitfield(m.Data)

	case protocol.MsgRequest:
		if peer.Pieces == nil {
			// Leecher ignores
			return
		}
		idx := int(binary.BigEndian.Uint32(m.Data)) // 4-byte index
		piece := peer.Pieces[idx]
		resp := protocol.Message{
			ID:   protocol.MsgPiece,
			Data: append(Uint32ToBytes(uint32(idx)), piece...),
		}
		peer.SendCh <- resp

	case protocol.MsgPiece:
		// idx := int(binary.BigEndian.Uint32(m.Data[:4]))
		// data := m.Data[4:]
		// Verify against .bit hashes
		// if sha1.Sum(data) != expectedHash[idx] {
		// 	log.Printf("Bad hash for piece %d\n", idx)
		// 	return
		// }
		// savePiece(idx, data)
	}
}

func Uint32ToBytes(n uint32) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], n)
	return b[:]
}
