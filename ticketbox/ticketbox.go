package ticketbox

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	registry sync.Map
)

func init() {
	go run()
}

func run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		now := <-ticker.C
		registry.Range(func(key, value interface{}) bool {
			box := value.(*Box)
			if now.Unix()%box.interval == 0 {
				atomic.StoreInt64(&(box.current), box.max)
			}
			return true
		})
	}
}

// Box 表示一个票箱
type Box struct {
	max      int64
	current  int64
	interval int64 // seconds for resetting
}

// NewBox 创建一个新的票箱
func NewBox(max, interval int64) *Box {
	return &Box{
		current:  max,
		max:      max,
		interval: interval,
	}
}

// Get a ticket from box
func (box *Box) Get() (int64, error) {
	n := atomic.AddInt64(&(box.current), -1)
	if n < 0 {
		return -1, errors.New("no ticket")
	}
	return n, nil
}

// Get 获取一张 key 的票根，参数 max 和 interval 指定了最多为该 key 在 interval 时间内发出 max 个票根。
// 如果超出了返回 error, 如果未超出则返回非负数的票号
func Get(key string, max int64, interval int64) (int64, error) {
	box := NewBox(max, interval)
	inter, _ := registry.LoadOrStore(key, box)
	return inter.(*Box).Get()
}
