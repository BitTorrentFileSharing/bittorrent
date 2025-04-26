// UDP message

package dht

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
)

type Msg struct {
	T        string    `json:"t"`              // ping, pong, announce, findPeers, peers
	ID       string    `json:"id"`             // hex
	Info     string    `json:"info,omitempty"` // Hex of infoHash
	Addr     string    `json:"addr,omitempty"`
	TcpList  []string  `json:"tcp_list,omitempty"`  // list of tcp addresses of seeders
	DHTPeers []MsgPeer `json:"dht_peers,omitempty"` // list of udp addresses of dht nodes
}

// Only for messages, I parse it into table.Peer object later
type MsgPeer struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

// Serializes message and send it via UDP to address
func send(conn *net.UDPConn, addr *net.UDPAddr, m Msg) error {
	data, err := json.Marshal(&m)
	if err != nil {
		return err
	}

	// Emit a line before the send
	_, err = conn.WriteToUDP(data, addr)
	logger.Log("udp_send", map[string]any{
		"to":   addr.String(),
		"type": m.T,
		"size": len(data),
	})
	return err
}

// Reads UDP message and attempt to decode it as Msg.
func recv(conn *net.UDPConn) (Msg, *net.UDPAddr, error) {
	var msg Msg
	buf := make([]byte, 1024)

	n, addr, err := conn.ReadFromUDP(buf)
	if err != nil {
		return msg, nil, err
	}

	if n == 0 {
		return msg, addr, errors.New("received empty packet")
	}

	err = json.Unmarshal(buf[:n], &msg)
	if err != nil {
		return msg, addr, err
	}

	logger.Log("udp_recv", map[string]any{
		"from": addr.String(),
		"type": msg.T,
		"size": n,
	})

	return msg, addr, err
}
