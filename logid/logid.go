package logid

import (
	"fmt"
	"time"

	"github.com/cloudfly/go/tools"
)

func String(prefix ...string) string {
	now := time.Now()
	timestr := fmt.Sprintf("%02d%02d%02d%02d%02d%02d", now.Year()%100, now.Month(), now.Day(), now.Hour(), now.Minute(), now.Second())
	// b does not escape for const length=53
	b := make([]byte, 0, 32)
	if len(prefix) > 0 {
		b = append(b, prefix[0]...)
	}
	b = append(b, timestr...)
	b = append(b, tools.Random(8)...)
	return string(b)
}
