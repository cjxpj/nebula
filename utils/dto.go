package utils

import "sync"

var (
	fileMutex sync.RWMutex
)

// file
type FileQueue struct {
	FileName string
}
