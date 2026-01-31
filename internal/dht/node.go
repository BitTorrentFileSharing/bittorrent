package dht

import (
	"encoding/hex"
	"fmt"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
)

const (
	inboxBufferSize     = 32
	inboxPeerBufferSize = 8
	maxReplyNodes       = 10
	findPeersTimeout    = 500 * time.Millisecond
	idLen               = 20
)

// Node represents a DHT node with its ID, connection and routing table.
type Node struct {
	ID           [idLen]byte         // Node ID (SHA-1)
	Conn         *net.UDPConn        // UDP conn for communication
	RoutingTable *Table              // Contains known peers
	Seeds        map[string][]string // InfoHash -> []tcpAddr
	inbox        chan packet         // Channel of incoming UDP messages

	// Same inbox, but for different message types that need to be isolated
	inboxPeer chan packet
}

type packet struct {
	msg Msg
	adr *net.UDPAddr
}

// New creates and starts a new DHT node listening on the specified address.
func New(listen string) (*Node, error) {
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

	node := &Node{
		ID:           id,
		Conn:         conn,
		RoutingTable: NewTable(id),
		Seeds:        make(map[string][]string),
		inbox:        make(chan packet, inboxBufferSize),
		inboxPeer:    make(chan packet, inboxPeerBufferSize),
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

// udpLoop is the single UDP reader goroutine.
func (node *Node) udpLoop() {
	for {
		msg, adr, err := recv(node.Conn)
		if err != nil {
			logger.Log("UDP_recv_error", map[string]any{"error": err.Error()})

			continue // Silently ignore incoming errors
		}

		node.inbox <- packet{msg, adr}
	}
}

func (node *Node) dispatchLoop() {
	for p := range node.inbox {
		// Need to handle peers isolated
		if p.msg.T == "peers" {
			node.inboxPeer <- p
		} else {
			node.handle(p.msg, p.adr)
		}
	}
}

func (node *Node) handle(msg Msg, adr *net.UDPAddr) {
	// Refresh routing table with sender's node-ID
	if raw, err := hex.DecodeString(msg.ID); err == nil && len(raw) == idLen {
		var id20 [idLen]byte

		copy(id20[:], raw)
		node.RoutingTable.Update(Peer{ID: id20, Addr: adr})
	}

	switch msg.T {
	case "ping":
		node.handlePing(adr)
	case "pong":
		node.handlePong(msg)
	case "announce":
		node.handleAnnounce(msg)
	case "findPeers":
		node.handleFindPeers(msg, adr)
	}
}

func (node *Node) handlePing(adr *net.UDPAddr) {
	// Collect peers and send them
	peers := node.RoutingTable.GetNPeers(maxReplyNodes)
	dhtPeers := make([]MsgPeer, 0, len(peers))
	addresses := make([]string, 0, len(peers))

	for _, n := range peers {
		idHex := hex.EncodeToString(n.ID[:])
		addrStr := n.Addr.String()

		dhtPeers = append(dhtPeers, MsgPeer{
			ID:   idHex,
			Addr: addrStr,
		})
		addresses = append(addresses, addrStr)
	}

	_ = send(node.Conn, adr, Msg{
		T:        "pong",
		ID:       hex.EncodeToString(node.ID[:]),
		DHTPeers: dhtPeers,
	})

	logger.Log("sent_ping_ponged_peers", map[string]any{"peers": addresses})
}

func (node *Node) handlePong(msg Msg) {
	msgPeers := msg.DHTPeers
	for _, msgPeer := range msgPeers {
		udpAddr, err := net.ResolveUDPAddr("udp4", msgPeer.Addr)
		if err != nil {
			logger.Log("bad_address", map[string]any{
				"addr": msgPeer.Addr,
				"err":  err.Error(),
			})

			continue
		}

		rawID, err := hex.DecodeString(msgPeer.ID)
		if err != nil || len(rawID) != idLen {
			continue
		}

		var id20 [idLen]byte
		copy(id20[:], rawID)

		node.RoutingTable.Update(Peer{
			ID:   id20,
			Addr: udpAddr,
		})
	}
}

func (node *Node) handleAnnounce(msg Msg) {
	if msg.Addr != "" {
		addTCP(node.Seeds, msg.Info, msg.Addr)
	}

	out := make([]string, 0, len(node.Seeds))
	for infoHash, peers := range node.Seeds {
		out = append(out, fmt.Sprintf("%s: [%s]",
			infoHash,
			strings.Join(peers, ", "),
		))
	}

	logger.Log("AVAILABLE_SEEDERS", map[string]any{"seeders": out})
}

func (node *Node) handleFindPeers(msg Msg, adr *net.UDPAddr) {
	list := slices.Clone(node.Seeds[msg.Info])
	list = deduplicate(list)

	logger.Log("Answer to findPeers", map[string]any{"seeders": list})

	_ = send(node.Conn, adr, Msg{
		T:       "peers",
		ID:      hex.EncodeToString(node.ID[:]),
		Info:    msg.Info,
		TCPList: list,
	})
}

// Ping sends a ping message to the given address (expects pong).
func (node *Node) Ping(addr string) {
	resolvedAddr, _ := net.ResolveUDPAddr("udp4", addr)
	_ = send(node.Conn, resolvedAddr, Msg{
		T:  "ping",
		ID: hex.EncodeToString(node.ID[:]),
	})
}

// Announce tells every known DHT neighbor (UDP) that
// "I serve infoHash and you can fetch the file from tcpAddr".
func (node *Node) Announce(hexInfoHash, tcpContact string) {
	msg := Msg{
		T:    "announce",
		ID:   hex.EncodeToString(node.ID[:]),
		Info: hexInfoHash,
		Addr: tcpContact,
	}

	// Sends this message for each known node
	for _, bucket := range node.RoutingTable.bucket {
		for _, peer := range bucket.peers {
			_ = send(node.Conn, peer.Addr, msg)
		}
	}
}

// FindPeers sends a single findPeers query to bootstrap and waits
// for a corresponding "peers" reply. It returns the list
// of TCP addresses contained in that reply.
func (node *Node) FindPeers(bootstrap string, infoHex string) []string {
	// 1. Send query
	adr, err := net.ResolveUDPAddr("udp4", bootstrap)
	if err != nil {
		logger.Log("findPeers_bad_address", map[string]any{"err": err.Error()})

		return nil
	}

	_ = send(node.Conn, adr, Msg{
		T:    "findPeers",
		ID:   hex.EncodeToString(node.ID[:]),
		Info: infoHex,
	})

	timeout := time.After(findPeersTimeout)
	for {
		select {
		case p := <-node.inboxPeer:
			// Skip other messages
			if p.msg.T == "peers" {
				return p.msg.TCPList
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

func deduplicate(in []string) []string {
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
