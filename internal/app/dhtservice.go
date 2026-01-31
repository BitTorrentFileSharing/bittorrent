package app

import (
	"encoding/hex"
	"strings"

	"github.com/BitTorrentFileSharing/bittorrent/internal/dht"
	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
)

// DHTService wraps a DHT node for peer discovery.
type DHTService struct {
	Node *dht.Node
}

// StartDHT creates a UDP node and kicks off bootstrap pings.
func StartDHT(listen string, bootstrapCSV string) (*DHTService, error) {
	if listen == "" { // User disabled DHT
		logger.Log("dht_disabled", nil)

		return nil, nil
	}

	dhtNode, err := dht.New(listen)
	if err != nil {
		return nil, err
	}

	// Bootstrap new nodes in background
	go func() {
		for host := range strings.SplitSeq(bootstrapCSV, ",") {
			if host != "" {
				dhtNode.Ping(host)
			}
		}
	}()

	return &DHTService{Node: dhtNode}, nil
}

// LookupPeers searches for peers serving the given infoHash.
func (svc *DHTService) LookupPeers(infoHash [20]byte) []string {
	if svc == nil {
		return nil
	}

	const (
		alpha     = 10 // num of parallel queries
		maxRounds = 10 // iterative depth
		maxPeers  = 50 // stop early criteria
	)

	hexedInfoHash := hex.EncodeToString(infoHash[:])

	seen := map[string]struct{}{} // Just a set
	queue := svc.Node.RoutingTable.Closest(infoHash, alpha)

	// START LOGS
	dhtAddresses := make([]string, 0, len(queue))
	for _, d := range queue {
		dhtAddresses = append(dhtAddresses, d.Addr.String())
	}

	logger.Log("leecher_peers_lookup", map[string]any{"available_dhts": dhtAddresses})
	// END LOGS

	for round := 0; round < maxRounds && len(queue) > 0 && len(seen) < maxPeers; round++ {
		target := queue[0]
		queue = queue[1:]

		// 1. Send FIND PEERS message
		reply := svc.Node.FindPeers(target.Addr.String(), hexedInfoHash)
		logger.Log("dht_lookup_reply",
			map[string]any{"from": target.Addr.String(), "peers": reply})

		// Add strings to map
		for _, addr := range reply {
			if _, ok := seen[addr]; ok {
				continue
			}

			seen[addr] = struct{}{}
		}
	}

	// Convert map -> slice
	out := make([]string, 0, len(seen))
	for peer := range seen {
		out = append(out, peer)
	}

	return out
}

// Announce tells DHT peers that we serve the given infoHash at tcpAddr.
func (svc *DHTService) Announce(infoHash [20]byte, tcpAddr string) {
	if svc == nil {
		return
	}

	hexInfoHash := hex.EncodeToString(infoHash[:])
	svc.Node.Announce(hexInfoHash, tcpAddr)
}
