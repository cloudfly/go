package consishash

import (
	"fmt"
	"hash/fnv"
	"testing"
	"time"
)

func getInitNodes() []string {
	return []string{
		"127.0.0.1",
		"192.168.1.1",
		"10.1.1.1",
		"10.1.2.1",
		"10.1.3.1",
		"10.1.4.1",
		"10.1.5.1",
		"10.1.6.1",
		"10.1.7.1",
		"10.1.2.2",
		"10.1.3.2",
		"10.1.4.2",
		"10.1.5.2",
		"10.1.6.2",
		"10.1.7.2",
		"10.1.2.3",
		"10.1.3.3",
		"10.1.4.3",
		"10.2.5.3",
		"10.2.6.3",
		"10.2.7.3",
		"10.2.2.3",
		"10.2.3.3",
		"10.2.4.3",
		"10.2.5.3",
		"10.2.6.3",
		"10.2.7.3",
		"1x.2.5.3",
		"1x.2.6.3",
		"1x.2.7.3",
		"1x.2.2.3",
		"1x.2.3.3",
		"1x.2.4.3",
		"1x.2.5.3",
		"1x.2.6.3",
		"1x.2.7.3",
	}
}

func TestHash(t *testing.T) {
	nodes := getInitNodes()

	hash := NewHash(nodes)
	data := make(map[string]string)

	for i := 0; i < 30000; i++ {
		key := fmt.Sprintf("%d", time.Now().UnixNano())
		node := hash.Get(key)
		value, _ := data[key]

		if value == "" {
			data[key] = node.ID
		} else if value != node.ID {
			t.Error(key, "get", node.ID, "but before we got", value)
		}
	}

	hash.AddNode(Node{ID: "24.21.11.1"})

	failed := 0
	for key, value := range data {
		node := hash.Get(key)
		if value != node.ID {
			failed++
		}
	}
	fmt.Printf("failed ratio %.2f%%\n", float64(failed)/30000*100)
}

func TestHashCommon(t *testing.T) {
	hash := func(key string) uint32 {
		f := fnv.New32()
		f.Write([]byte(key))
		return f.Sum32()
	}

	nodes := getInitNodes()

	data := make(map[string]string)

	for i := 0; i < 30000; i++ {
		key := fmt.Sprintf("%d", time.Now().UnixNano())
		node := nodes[hash(key)%uint32(len(nodes))]
		value, _ := data[key]
		if value == "" {
			data[key] = node
		} else if value != node {
			t.Error(key, "get", node, "but before we got", value)
		}
	}

	nodes = append(nodes, "24.21.11.1")

	failed := 0

	for key, value := range data {
		node := nodes[hash(key)%uint32(len(nodes))]
		if value != node {
			failed++
		}
	}
	fmt.Printf("failed ratio %.2f%%\n", float64(failed)/30000*100)
}
