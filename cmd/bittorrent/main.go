package main

import (
	"flag"
	"log"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/BitTorrentFileSharing/bittorrent/internal/logger"
	"github.com/BitTorrentFileSharing/bittorrent/internal/metainfo"
	"github.com/BitTorrentFileSharing/bittorrent/internal/peer"
	"github.com/BitTorrentFileSharing/bittorrent/internal/protocol"
	"github.com/BitTorrentFileSharing/bittorrent/internal/storage"
	"github.com/BitTorrentFileSharing/bittorrent/internal/util"
)

var (
	seed     = flag.String("seed", "", "Path to original file to seed")
	get      = flag.String("get", "", "Path to .bit metadata to download")
	addr     = flag.String("addr", ":6881", "listen address")
	peerList = flag.String("peer", "", "comma-separated peers (host:port,host2:port2)")
	dest     = flag.String("dest", ".", "Destination folder for download file (leecher)")
)

func main() {
	flag.Parse()
	switch {
	case *seed != "":
		runSeeder()
	case *get != "":
		runLeecher()
	default:
		log.Fatal("use -seed FILE or -get META")
	}
}

func runSeeder() {
	dataPath := *seed
	metaPath := dataPath + ".bit"

	// 1. Create or Load MetaInfo
	var meta *metainfo.Meta
	if util.Exists(metaPath) {
		// Already seeded there.
		m, err := metainfo.Load(metaPath)
		if err != nil {
			log.Fatal(err)
		}
		meta = m
	} else {
		// First time. Split file, Compute Hashes, save .bit
		pieces, hashes, err := storage.Split(dataPath, storage.DefaultPiece)
		if err != nil {
			log.Fatal(err)
		}
		meta = &metainfo.Meta{
			FileName:   filepath.Base(dataPath),
			FileLength: int64(len(pieces) * storage.DefaultPiece),
			PieceSize:  storage.DefaultPiece,
			Hashes:     hashes,
		}
		if err := meta.Write(metaPath); err != nil {
			log.Fatal(err)
		}
		log.Println("[+] wrote", metaPath)
	}

	// Need to always keep pieces in memory for uploads
	// Split again if we did not just create them
	pieces, _, err := storage.Split(dataPath, storage.DefaultPiece)
	if err != nil {
		log.Fatal(err)
	}

	// 2. Build local state
	bitfield := storage.NewBitfield(len(meta.Hashes))
	for i := range bitfield {
		bitfield.Set(i) // Seeder owns everything
	}
	peerID := protocol.RandomPeerID()

	// 3. Start Listening
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	log.Println("[*] Seeder listening on:", *addr)

	infoHash, _ := protocol.InfoHash(metaPath) // 20-byte SHA-1

	for {
		conn, err := ln.Accept()
		log.Println("[*] New connection:", conn.RemoteAddr().String())
		if err != nil {
			log.Println("[!] accept error:", err)
			continue
		}
		p := peer.New(conn, bitfield, peerID)
		p.Pieces = pieces // allow uploads

		// Send handshake + bitfield
		p.SendCh <- protocol.NewHandshake(infoHash[:], peerID[:])
		p.SendCh <- protocol.NewBitfield(bitfield)
	}
}

// Does many things...
func runLeecher() {
	// 0. Load metadata (.bit file)
	meta, err := metainfo.Load(*get)
	if err != nil {
		log.Fatalf("[!] failed to load metadata file %q: %v", *get, err)
	}
	log.Printf("[+] meta ok - %d pieces, pieceSize=%d",
		len(meta.Hashes), meta.PieceSize)

	// 1. Local State shared by ALL connections
	bitfield := storage.NewBitfield(len(meta.Hashes)) // zeroes since we own nothing
	pieces := make([][]byte, len(meta.Hashes))        // download buffer
	availability := make([]int, len(meta.Hashes))     // how many peers have each piece
	missing := make([]bool, len(meta.Hashes))         // True=Need it
	for i := range missing {
		missing[i] = true
	}

	var (
		peers   []*peer.Peer
		peersMu sync.Mutex
	)

	// 2. Helper functions
	// Counts how many peers own the following piece
	updateAvailability := func() {
		for i := range availability {
			availability[i] = 0
		}
		peersMu.Lock()
		// log.Println("[?] UpdAv: Update availability mask...")
		for _, p := range peers {
			// log.Printf("[?] UpdAv: Checking what owns Peer %d (%s)", num, p.Conn.RemoteAddr().String())
			for i := range availability {
				if p.Bitfield.Has(i) {
					availability[i]++
					// log.Printf("[?] UpdAv: Peer %d has %d piece (%s)", num, i, p.Conn.RemoteAddr().String())
				}
			}
		}
		// log.Println("[?] UpdAv: Finished updating availability mask.")
		peersMu.Unlock()
	}

	// Finds a target peer that has a piece
	// and then asks him to send this piece
	requestPiece := func(idx int) {
		log.Printf("[?] ReqP: Requesting %d piece...", idx)
		peersMu.Lock()
		var target *peer.Peer
		for _, p := range peers {
			if p.Bitfield.Has(idx) {
				target = p
				log.Printf("[?] ReqP: Found peer with asked piece %s", target.Conn.RemoteAddr().String())
				break
			}
		}
		peersMu.Unlock()
		if target == nil {
			// Nobody owns it yet
			log.Printf("[?] ReqP: Did not found satisfying peers. Leaving.")
			return
		}
		requestMsg := protocol.Message{
			ID: protocol.MsgRequest,
			Data: append(util.Uint32ToBytes(uint32(idx)),
				util.Uint32ToBytes(0)...), // offset 0
		}
		target.SendCh <- requestMsg
		logger.Log(
			"request",
			map[string]any{
				"pieceIndex": idx,
				"targetPeer": target.Conn.RemoteAddr().String()})
	}

	// Updates pieces availability by going through all peers
	// and requests rarest piece from them.
	pickAndRequestNext := func() {
		// Count piece references amount
		updateAvailability()
		log.Printf("[?] PickNReq: Started searching rarest piece.")
		best := -1
		for i, need := range missing {
			if !need {
				continue
			}
			if best == -1 || availability[i] < availability[best] {
				best = i
			}
		}
		if best != -1 {
			// log.Printf("[?] PickNReq: Requesting rarest piece %d", best)
			requestPiece(best)
		} else {
			// log.Printf("[?] PickNReq: No need to request any pieces. Already done")
		}
	}

	pickerQuit := make(chan struct{}) // For blocking functional
	
	// Callback for peer
	onPieceDone := func(idx int) {
		// Piece not missing no more
		missing[idx] = false

		// broadcast haveMsg to every other peer
		haveMsg := protocol.Message{
			ID:   protocol.MsgHave,
			Data: util.Uint32ToBytes(uint32(idx)),
		}
		peersMu.Lock()
		for _, p := range peers {
			// Skip if they already have it
			if !p.Bitfield.Has(idx) {
				p.SendCh <- haveMsg
			}
		}
		peersMu.Unlock()

		// Existing progress / completion logic below
		// Log done pieces
		done := 0
		for _, need := range missing {
			if !need {
				done++
			}
		}
		logger.Log(
			"have",
			map[string]any{"pieceIndex": idx, "doneAmount": done})

		// Finished? Leave
		if done == len(missing) {
			outPath := filepath.Join(*dest, meta.FileName)
			if err := storage.Join(pieces, outPath); err != nil {
				log.Fatal(err)
			}
			logger.Log(
				"complete",
				map[string]any{"file": outPath})
			close(pickerQuit)
			return
		}
		// Request next to continue the flow of downloading
		log.Printf("[*] NEXT PNREQ IN FLOW")
		pickAndRequestNext()
	}

	// 3. Dial with every peer in -peer list
	infoHash, _ := protocol.InfoHash(*get)

	for addr := range strings.SplitSeq(*peerList, ",") {
		addr = strings.TrimSpace(addr)
		if addr == "" {
			continue
		}

		go func(remote string) {
			conn, err := net.Dial("tcp", remote)
			if err != nil {
				log.Fatalf("[!] dial %s: %v", remote, err)
				return
			}
			logger.Log("join", map[string]any{"peer": remote})

			p := peer.New(conn, bitfield, protocol.RandomPeerID())
			p.Meta = meta
			p.Pieces = pieces // Share same buffer!

			// Link piece-done callback and leave detection
			p.OnHave = func(idx int) {
				if idx == -1 { // Peer closed
					logger.Log(
						"leave",
						map[string]any{"peer": remote},
					)
					return
				}
				onPieceDone(idx)
			}

			// Handshake + empty bitfield
			p.SendCh <- protocol.NewHandshake(infoHash[:], p.ID[:])
			logger.Log(
				"handShake",
				map[string]any{},
			)

			p.SendCh <- protocol.NewBitfield(bitfield)
			logger.Log(
				"newBitfield",
				map[string]any{},
			)

			// Remember that peer
			peersMu.Lock()
			peers = append(peers, p)
			peersMu.Unlock()

			// After join, try to request something it has
			log.Printf("[*] REQ AFTER JOINING")
			pickAndRequestNext()
		}(addr)
	}

	// 4. Background ticker: re-issue rarest-first every 2s
	go func() {
		tk := time.NewTicker(2 * time.Second)
		defer tk.Stop()
		for {
			select {
			case <-pickerQuit:
				return
			case <-tk.C:
				log.Printf("[*] TICKER MADE PnReq")
				pickAndRequestNext()
			}
		}
	}()

	// 5. Start by requesting something right away
	log.Printf("[*] FIRST PNREQ")
	pickAndRequestNext()

	// 6. Block until torrent finished
	<-pickerQuit
	time.Sleep(300 * time.Millisecond) // For final logs
}
