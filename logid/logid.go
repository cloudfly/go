package logid

import (
	"hash/crc32"
	"os"
	"strconv"
	"sync/atomic"
	"time"
)

const (
	machinePosition uint64 = 0xffff000000000000
	pidCodePosition uint64 = 0x0000ff0000000000
	secondPosition  uint64 = 0x000000ffffff0000
	incPosition     uint64 = 0x000000000000ffff
)

var (
	machineCode uint64
	pidCode     uint64
	inc         uint64
)

func init() {
	hostname, _ := os.Hostname()
	pid := os.Getpid()
	m32Code := crc32.ChecksumIEEE([]byte(hostname + strconv.Itoa(pid)))
	machineCode = uint64(m32Code)
	pidCode = uint64(pid)
}

// New generate a log id
// machineCode: 16bit
// pidCode: 8bit
// time: 24bit
// inc: 16bit
func New() uint64 {
	now := time.Now().Unix()
	inc := atomic.AddUint64(&inc, 1)
	ts := uint64(now)
	var ret uint64
	ret = (machineCode << 48) & machinePosition
	ret = ret | ((pidCode << 40) & pidCodePosition)
	ret = ret | ((ts << 16) & secondPosition)
	ret = ret | (inc & incPosition)
	return ret
}
