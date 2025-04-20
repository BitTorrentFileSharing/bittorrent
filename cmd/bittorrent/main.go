package main

import (
	"flag"
	"log"
	"net"
	"path/filepath"
	"time"

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
	peerList = flag.String("peer", "", "comma-separated list of peers to dial (host:port)")
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
	log.Println("Seeder listening on:", *addr)

	infoHash, _ := protocol.InfoHash(metaPath) // 20-byte SHA-1

	for {
		conn, err := ln.Accept()
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

// Downloads ALL pieces from one peer (the seeder)
// and writes final payload next to the .bit file when finished
func runLeecher() {
	// 1. Load metadata (.bit file)
	meta, err := metainfo.Load(*get) // --get movie.bin.bit
	if err != nil {
		log.Fatalf("[!] failed to load metadata file %q: %v", *get, err)
	}
	log.Println("[+] meta loaded: pieces =", len(meta.Hashes))

	// 2. Dial the seeder (single peer right now)
	conn, err := net.Dial("tcp", *peerList) // --peer localhost:6881
	if err != nil {
		log.Fatalf("[!] dial %s: %v", *peerList, err)
	}
	log.Println("[+] TCP connected to", *peerList)

	// 3. Local bitfield = all zero (we own nothing yet)
	bitfield := storage.NewBitfield(len(meta.Hashes))
	peerID := protocol.RandomPeerID()

	// 4. Create peer object
	newPeer := peer.New(conn, bitfield, peerID)
	newPeer.Meta = meta
	newPeer.Pieces = make([][]byte, len(meta.Hashes)) // download buffer

	// PROCEED WITH HANDSHAKE + BITFIELD
	infoHash, _ := protocol.InfoHash(*get)
	newPeer.SendCh <- protocol.NewHandshake(infoHash[:], peerID[:])
	newPeer.SendCh <- protocol.NewBitfield(bitfield)
	log.Println("[+] handshake + empty bitfield sent")

	// 5. Piece-picker state
	missing := make([]bool, len(meta.Hashes)) // true -> need piece
	pickerQuit := make(chan struct{})
	for i := range missing {
		missing[i] = true
	}

	// Helper: request next missing piece sequentially
	// sends MsgRequest. Seeder responds with MsgPiece
	requestNext := func() {
		for idx, need := range missing {
			if need {
				req := protocol.Message{
					ID: protocol.MsgRequest,
					Data: append(util.Uint32ToBytes(uint32(idx)), // piece index
						util.Uint32ToBytes(0)...), // offset = 0 (always?)
				}
				newPeer.SendCh <- req
				log.Printf("[*] requested piece %d", idx)
				return
			}
		}
	}

	// 6. Define HOOK
	// This hook marks pieces done, logs,
	// either requestsNext() or completes.
	newPeer.OnHave = func(idx int) {
		missing[idx] = false

		// Progress log
		done, total := 0, len(missing)
		for _, need := range missing {
			if !need {
				done++
			}
		}
		log.Printf("[+] have piece %d (%d/%d)", idx, done, total)

		// Completed torrent?
		if done == total {
			log.Println("[*] all pieces verified, writing final file ...")
			out := filepath.Join(*dest, meta.FileName)
			if err := storage.Join(newPeer.Pieces, out); err != nil {
				log.Fatal("[!] join:", err)
			}
			log.Println("[*] File written!", out)
			close(pickerQuit)
			return
		}
		// Otherwise ask for the next missing piece
		requestNext()
	}

	// 7. Start the initial request (piece 0), then let callback drive flow
	requestNext()

	// 8. Block until torrent finished
	<-pickerQuit
	time.Sleep(500 * time.Microsecond) // For final logs
}
