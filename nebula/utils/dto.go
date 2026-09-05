package utils

import "sync"

// fileLocks 按文件路径维护独立的读写锁，替代原先所有文件共用的全局锁，
// 使不同文件路径之间的读写互不阻塞。
var (
	fileLocksMu sync.Mutex
	fileLocks   = map[string]*sync.RWMutex{}
)

// fileLock 返回指定文件路径对应的读写锁（按路径隔离）。
func fileLock(path string) *sync.RWMutex {
	fileLocksMu.Lock()
	defer fileLocksMu.Unlock()
	if l, ok := fileLocks[path]; ok {
		return l
	}
	l := &sync.RWMutex{}
	fileLocks[path] = l
	return l
}

// file
type FileQueue struct {
	FileName string
}
