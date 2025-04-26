package dht

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"slices"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
)

// DHT node with its id, connection and routingTable
type DHTNode struct {
	ID           [20]byte            // Node ID (SHA-1)
	Conn         *net.UDPConn        // UDP conn for communication
	RoutingTable *Table              // Contains known peers
	Seeds        map[string][]string // InfoHash -> []tcpAddr
	inbox        chan packet         // Channel of incoming UDP messages

	// Same inbox, but for different messages type that need to be isolated
	inboxPeer chan packet
}

type packet struct {
	msg Msg
	adr *net.UDPAddr
}

// Creates and start a new DHT node listening on a specified address.
func New(listen string) (*DHTNode, error) {
	addr, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return nil, err
	}

	// Generate a basic node ID
	id := protocol.RandomPeerID()

	node := &DHTNode{
		ID:           id,
		Conn:         conn,
		RoutingTable: NewTable(id),
		Seeds:        make(map[string][]string),
		inbox:        make(chan packet, 32),
		inboxPeer:    make(chan packet, 8),
	}

	logger.Log(
		"dht_started_listening",
		map[string]any{"addr": addr.String()},
	)
	// Each DHT server runs this loop
	go node.udpLoop()      // Socket loop
	go node.dispatchLoop() // message handler

	return node, nil
}

// 1. Single UDP reader goroutine
func (node *DHTNode) udpLoop() {
	for {
		msg, adr, err := recv(node.Conn)
		if err != nil {
			logger.Log("UDP_recv_error", map[string]any{"error": err.Error()})
			continue // Silently ignore incoming errors
		}
		node.inbox <- packet{msg, adr}
	}
}

func (node *DHTNode) dispatchLoop() {
	for p := range node.inbox {
		// Need to handle peers isolated
		if p.msg.T == "peers" {
			node.inboxPeer <- p
		} else {
			node.handle(p.msg, p.adr)
		}
	}
}

func (node *DHTNode) handle(msg Msg, adr *net.UDPAddr) {
	// Refresh routing table with sender's node-ID
	if raw, err := hex.DecodeString(msg.ID); err == nil && len(raw) == 20 {
		var id20 [20]byte
		copy(id20[:], raw)
		node.RoutingTable.Update(Peer{ID: id20, Addr: adr})
	}

	switch msg.T {
	case "ping":
		// Collect peers and send them
		var dhtPeers []MsgPeer
		for _, node := range node.RoutingTable.GetNPeers(5) {
			dhtPeers = append(dhtPeers, MsgPeer{ID: hex.EncodeToString(node.ID[:]), Addr: node.Addr.String()})
		}
		
		send(node.Conn, adr, Msg{
			T: "pong",
			ID: hex.EncodeToString(node.ID[:]),
			DHTPeers: dhtPeers,
		})

		// LOGGER START
		var addresses []string
		for _, peer := range dhtPeers {
			addresses = append(addresses, peer.Addr)
		}
		logger.Log("sent_ping_ponged_peers", map[string]any{"peers": addresses})
		// LOGGER END

	case "pong":
		// take UDP nodes there
		msgPeers := msg.DHTPeers
		for _, msgPeer := range msgPeers {
			udpAddr, err := net.ResolveUDPAddr("udp4", msgPeer.Addr)
			if err != nil {
				logger.Log("bad_address", map[string]any{"addr": msgPeer.Addr, "err": err.Error()})
			}
			rawID, err := hex.DecodeString(msgPeer.ID)
			if err != nil {
				logger.Log("bad_dht_peer", map[string]any{"err": err.Error()})
			}
			var id20 [20]byte
			copy(id20[:], rawID)

			peer := Peer{
				ID: id20,
				Addr: udpAddr,
			}
			node.RoutingTable.Update(peer)
		}

	case "announce":
		if msg.Addr != "" {
			addTCP(node.Seeds, msg.Info, msg.Addr)
		}

		// LOG INFORMATION
		// DELETE IN PRODUCTION
		var out []string
		for infoHash, peers := range node.Seeds {
			out = append(out, fmt.Sprintf("%s: [%s]",
				infoHash,
				strings.Join(peers, ", "),
			))
		}
		logger.Log("AVAILABLE_SEEDERS", map[string]any{"seeders": out})
		// LOG END

	case "findPeers":
		list := slices.Clone(node.Seeds[msg.Info]) // known seeders
		list = dedup(list)
		logger.Log("Answer to findPeers", map[string]any{"seeders": list})

		send(node.Conn, adr, Msg{
			T: "peers", ID: hex.EncodeToString(node.ID[:]),
			Info: msg.Info, TcpList: list,
		})
	}

}

/// Public Helpers

// Sends a ping message to the given address (expects pong)
func (node *DHTNode) Ping(addr string) {
	resolvedAddr, _ := net.ResolveUDPAddr("udp4", addr)
	send(node.Conn, resolvedAddr, Msg{
		T:  "ping",
		ID: hex.EncodeToString(node.ID[:]),
	})
}

// Announce tells every known DHT neighbor (UDP) that
// “I serve infoHash and you can fetch the file from tcpAddr”.
func (node *DHTNode) Announce(hexInfoHash, tcpContact string) {
	msg := Msg{
		T:    "announce",
		ID:   hex.EncodeToString(node.ID[:]),
		Info: hexInfoHash,
		Addr: tcpContact,
	}

	// Sends this message for each known node
	for _, bucket := range node.RoutingTable.bucket {
		for _, peer := range bucket.peers {
			send(node.Conn, peer.Addr, msg)
		}
	}
}

// Sends a single findPeers query to *bootstrap* and waits
// up to 500 ms for a corresponding "peers" reply. It returns the list
// of TCP addresses contained in that reply. (deduplicated by caller)
func (node *DHTNode) FindPeers(bootstrap string, infoHex string) []string {
	// 1. Send query
	adr, err := net.ResolveUDPAddr("udp4", bootstrap)
	if err != nil {
		logger.Log("findPeers_bad_address", map[string]any{"err": err.Error()})
		return nil
	}
	send(node.Conn, adr, Msg{
		T:    "findPeers",
		ID:   hex.EncodeToString(node.ID[:]),
		Info: infoHex,
	})

	timeout := time.After(500 * time.Millisecond)
	for {
		select {
		case p := <-node.inboxPeer:
			// Skip other messages
			if p.msg.T == "peers" {
				return p.msg.TcpList
			}
		case <-timeout:
			logger.Log("findPeers_timeout", map[string]any{"bootstrap": bootstrap})
			return nil
		}
	}
}

func addTCP(store map[string][]string, ih, tcp string) {
	if slices.Contains(store[ih], tcp) {
		return
	}
	store[ih] = append(store[ih], tcp)
}

func dedup(in []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}
