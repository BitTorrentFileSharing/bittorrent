package app

import (
	"encoding/hex"
	"log"
	"strings"

	"github.com/BitTorrentFileSharing/bittorrent/internal/dht"
	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
)

type DHTService struct {
	Node *dht.DHTNode
}

// Creates UDP node and kicks off bootstrap pings.
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

func (svc *DHTService) LookupPeers(infoHash [20]byte) []string {
	if svc == nil {
		return nil
	}

	const (
		alpha     = 3  // num of parallel queries (really it is num of peers)
		maxRounds = 3  // iterative depth
		maxPeers  = 50 // stop early criteria
	)

	hexedInfoHash := hex.EncodeToString(infoHash[:])
	
	seen := map[string]struct{}{} // Just a set
	queue := svc.Node.RoutingTable.Closest(infoHash, alpha)

	for round := 0; round < maxRounds && len(queue) > 0 && len(seen) < maxPeers; round++ {
		log.Println("Try number", round)
		
		target := queue[0]
		queue = queue[1:]

		// 1. Send FIND PEERS message
		reply := svc.Node.FindPeers(target.Addr.String(), hexedInfoHash)
		logger.Log("dht_lookup_reply",
			map[string]any{"from": target.Addr.String(), "peers": len(reply)})

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
	logger.Log("dht_lookup_done", map[string]any{"total": len(out)})
	return out
}

// Seeder side
func (svc *DHTService) Announce(infoHash [20]byte, tcpAddr string) {
	if svc == nil {
		return
	}
	hexInfoHash := hex.EncodeToString(infoHash[:])
	svc.Node.Announce(hexInfoHash, tcpAddr)
}
