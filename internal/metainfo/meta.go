package metainfo

import (
	"encoding/json"
	"os"
)

type Meta struct {
	FileName   string   `json:"name"`
	FileLength int64    `json:"length"`
	PieceSize  int      `json:"piece_size"`
	Hashes     [][]byte `json:"hashes"` // SHA-1 for each piece
}

// Saves the struct as a pretty JSON
func (m *Meta) Write(path string) error {
	b, err := json.MarshalIndent(m, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Parses file back into Meta
func Load(path string) (*Meta, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Meta
	return &m, json.Unmarshal(b, &m)
}
