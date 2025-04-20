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
	seed  = flag.String("seed", "", "Path to original file to seed")
	get   = flag.String("get", "", "Path to .bit metadata to download")
	addr  = flag.String("addr", ":6881", "listen address")
	peerL = flag.String("peer", "", "comma-separated list of peers to dial (host:port)")
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

func runLeecher() {
	// Load data
	meta, err := metainfo.Load(*get) // --get ~/file.bit
	if err != nil {
		log.Fatalf("[!] failed to load metadata file %q: %v", *get, err)
	}
	log.Println("[+] Successfully loaded meta")

	bitfield := storage.NewBitfield(len(meta.Hashes)) // all zeroes
	peerID := protocol.RandomPeerID()

	conn, err := net.Dial("tcp", *peerL) // pass --p localhost:6881
	if err != nil {
		log.Fatalln("Error while establishing connection:", err)
	}

	p := peer.New(conn, bitfield, peerID)
	expected := meta.Hashes // TODO: pass this into peer to verify
	log.Println("[+] Successfully established connection")

	infoHash, _ := protocol.InfoHash(*get)
	p.SendCh <- protocol.NewHandshake(infoHash[:], peerID[:])
	p.SendCh <- protocol.NewBitfield(bitfield)
	log.Println("[+] Successfully sent handshake and bitfield")

	// ONLY 1 PIECE FOR FUN TODO: ADD MORE
	req := protocol.Message{
		ID:   protocol.MsgRequest,
		Data: peer.Uint32ToBytes(0),
	}
	p.SendCh <- req
	log.Println("[*] Requested 1 chunk of data")

	// Wait a bit then exit (TODO: improve))
	time.Sleep(3 * time.Second)
	_ = expected // TODO: do some
}
