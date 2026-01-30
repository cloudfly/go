package consishash

import (
	"hash/fnv"
	"sync"
	"sync/atomic"
	"unsafe"
)

const (
	maxPosition = 2147483648
	numNodes    = 1024 // length of the hash ring, real node and virtual node
)

// Node represents the real node entity in hash ring
type Node struct {
	ID    string
	count int
}

// Hash 为一个带有一致性 hash 特性的群体
type Hash struct {
	rw        sync.RWMutex
	realNodes map[string]*Node
	nodes     [numNodes]*Node
}

// NewHash create a new consistant hash service
func NewHash(nodes []string) *Hash {
	hash := Hash{
		realNodes: make(map[string]*Node),
	}
	for i := range nodes {
		hash.realNodes[nodes[i]] = &Node{
			ID: nodes[i],
		}
	}
	i := 0
F:
	for {
		for _, node := range hash.realNodes {
			hash.nodes[i] = node
			node.count++
			i++
			if i >= numNodes {
				break F
			}
		}
	}

	return &hash
}

// Get will execute hash action, return the node that the value should belong to
func (hash *Hash) Get(key string) Node {
	f := fnv.New32()
	f.Write([]byte(key))
	node := unsafe.Pointer(hash.nodes[f.Sum32()%numNodes])
	hash.rw.RLock()
	defer hash.rw.RUnlock()
	return *(*Node)(atomic.LoadPointer(&node))
}

// AddNode 增加一个新节点
func (hash *Hash) AddNode(node Node) {
	node.count = 0
	nodep := &node
	hash.realNodes[node.ID] = nodep

	countLimit := numNodes / len(hash.realNodes)

	for i := 0; i < numNodes; i++ {
		tmp := hash.nodes[i]
		if tmp.count > countLimit {
			pointer := unsafe.Pointer(hash.nodes[i])
			atomic.StorePointer(&pointer, unsafe.Pointer(nodep))
			nodep.count++
			tmp.count--
		}
		if nodep.count >= countLimit {
			break
		}
	}
}

// RmNode 删除一个节点
func (hash *Hash) RmNode(id string) {

	hash.rw.Lock()
	defer hash.rw.Unlock()

	_, ok := hash.realNodes[id]
	if !ok {
		return
	}
	delete(hash.realNodes, id)

	i := 0
L:
	for {
		for _, node := range hash.realNodes {
			for {
				if i >= numNodes {
					break L
				}
				if hash.nodes[i].ID == id {
					hash.nodes[i] = node
					node.count++
					i++
					break
				} else {
					i++
				}
			}
		}
	}
}
