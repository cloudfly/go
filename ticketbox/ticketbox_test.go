package ticketbox

import (
	"testing"

	"github.com/cloudfly/go/test"
)

func TestBox(t *testing.T) {
	assert := test.Assert(t)
	n := 0
	for i := 0; i < 60; i++ {
		// 1分钟内最多发 10 票
		if _, err := Get("test", 10, 60); err == nil {
			n++
		}
	}
	assert.Equal(10, n)
}
