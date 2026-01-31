// Package dht implements a distributed hash table for peer discovery.
package dht

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
)

// Msg represents a DHT protocol message.
type Msg struct {
	T        string    `json:"t"`                  // ping, pong, announce, findPeers, peers
	ID       string    `json:"id"`                 // hex
	Info     string    `json:"info,omitempty"`     // Hex of infoHash
	Addr     string    `json:"addr,omitempty"`     // Address
	TCPList  []string  `json:"tcpList,omitempty"`  // list of tcp addresses of seeders
	DHTPeers []MsgPeer `json:"dhtPeers,omitempty"` // list of udp addresses of dht nodes
}

// MsgPeer represents a DHT peer in a message, containing ID and address.
type MsgPeer struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
}

// send serializes a message and sends it via UDP to the given address.
func send(conn *net.UDPConn, addr *net.UDPAddr, m Msg) error {
	data, err := json.Marshal(&m)
	if err != nil {
		return err
	}

	// Emit a line before the send
	_, err = conn.WriteToUDP(data, addr)
	if err != nil {
		logger.Log("udp_send_error", map[string]any{
			"to":   addr.String(),
			"type": m.T,
			"size": len(data),
			"err":  err.Error(),
		})

		return err
	}

	logger.Log("udp_send", map[string]any{
		"to":   addr.String(),
		"type": m.T,
		"size": len(data),
	})

	return nil
}

const udpBufferSize = 1024

// recv reads a UDP message and attempts to decode it as Msg.
func recv(conn *net.UDPConn) (Msg, *net.UDPAddr, error) {
	var msg Msg

	buf := make([]byte, udpBufferSize)

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
