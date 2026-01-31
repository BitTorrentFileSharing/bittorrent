// Package metainfo handles torrent metadata serialization.
package metainfo

import (
	"encoding/json"
	"fmt"
	"os"
)

// Meta represents torrent metadata including file info and piece hashes.
type Meta struct {
	FileName   string   `json:"name"`
	FileLength int64    `json:"length"`
	PieceSize  int      `json:"pieceSize"`
	Hashes     [][]byte `json:"hashes"` // SHA-1 for each piece
}

// Write saves the struct as JSON to the given path.
func (m *Meta) Write(path string) error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, b, 0o644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Load parses a JSON file back into a Meta struct.
func Load(path string) (*Meta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var m Meta

	return &m, json.Unmarshal(b, &m)
}
