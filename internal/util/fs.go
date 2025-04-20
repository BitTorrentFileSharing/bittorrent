package util

import "os"

// Checks if file exists on this path 
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
