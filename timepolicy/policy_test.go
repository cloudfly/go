package timepolicy

import (
	"context"
	"fmt"
	"runtime/debug"
	"testing"
	"time"

	"github.com/cloudfly/go/test"
)

func TestPolicy(t *testing.T) {
	assert := test.Assert(t)
	// 2019-11-16 15:54:33
	initTime := time.Date(2019, 11, 16, 15, 54, 23, 0, time.Local)
	now := time.Date(2019, 11, 16, 15, 57, 41, 0, time.Local)
	p, err := ParsePolicy(initTime, "1m")
	assert.NoError(err)
	next := p.NextTime(now.Unix())
	assert.Equal(now.Truncate(time.Minute).Add(time.Minute).Unix(), next)

	p, _ = ParsePolicy(initTime, "1s")
	assert.Equal(now.Unix(), p.NextTime(now.Unix()))

	p, err = ParsePolicy(initTime, "1m:10s")
	assert.NoError(err)
	next = p.NextTime(now.Unix())
	assert.Equal(time.Date(2019, 11, 16, 15, 57, 43, 0, time.Local).Unix(), next)

	next = p.NextTime(initTime.Add(-time.Minute).Unix())
	assert.Equal(initTime.Add(time.Minute).Unix(), next) // next 即是开始时间

	p, err = ParsePolicy(initTime, "1m:10s:10m")
	assert.NoError(err)
	next = p.NextTime(now.Unix())
	assert.Equal(time.Date(2019, 11, 16, 15, 57, 43, 0, time.Local).Unix(), next)

	next = p.NextTime(initTime.Add(-time.Minute).Unix())
	assert.Equal(initTime.Add(time.Minute).Unix(), next) // next 即是开始时间

	next = p.NextTime(initTime.Add(time.Hour).Unix())
	assert.Equal(int64(0), next)

	p, err = ParsePolicy(initTime, "1m:10s:10m,10m:3m")
	assert.NoError(err)
	next = p.NextTime(now.Unix())
	assert.Equal(time.Date(2019, 11, 16, 15, 57, 43, 0, time.Local).Unix(), next)

	next = p.NextTime(initTime.Add(-time.Minute).Unix())
	assert.Equal(initTime.Add(time.Minute).Unix(), next) // next 即是开始时间

	next = p.NextTime(now.Add(time.Hour).Unix())
	// 走 10m:3m 策略, 2019-11-16 16:04:23 开始每 3m 一次
	// now.Add(time.Hour) 是否 2019-11-16 16:57:41
	// 16:04:23 + 3m * 18 => 16:04:23 + 54m => 16:58:23
	assert.Equal(time.Date(2019, 11, 16, 16, 58, 23, 0, time.Local).Unix(), next)
}

type MockJob struct {
	prefix   string
	t        *testing.T
	from     time.Time
	finished bool
}

func (job *MockJob) Do(t time.Time) {
	fmt.Println(job.prefix, t.Format("2006-01-02T15:04:05"))
}

func (job *MockJob) Finished() bool {
	return job.finished
}

func TestEngine(t *testing.T) {
	assert := test.Assert(t)
	engine := NewEngine(context.Background())
	job := &MockJob{
		prefix: "[one]",
		from:   time.Now(),
		t:      t,
	}
	err := engine.RegisterWithTime(job.from, ":2s:5s,6s:3s:10m", job)
	assert.NoError(err)
	time.Sleep(time.Second * 15)
	job2 := &MockJob{
		prefix: "[two]",
		from:   time.Now(),
		t:      t,
	}
	job.finished = true
	err = engine.RegisterWithTime(job.from, "1s", job2)
	assert.NoError(err)
	time.Sleep(time.Second * 4)
}

func TestParsePolicyItem(t *testing.T) {
	assert := test.Assert(t)
	now := time.Now()
	item, err := parsePolicyItem(now, []byte("2s"))
	assert.NoError(err)
	assert.Equal(int64(2), item.Interval)

	item, err = parsePolicyItem(now, []byte(":2s:"))
	assert.NoError(err)
	assert.Equal(int64(2), item.Interval)

	item, err = parsePolicyItem(now, []byte(":2s"))
	assert.NoError(err)
	assert.Equal(int64(2), item.Interval)

	item, err = parsePolicyItem(now, []byte("10s:2s"))
	assert.NoError(err)
	assert.Equal(now.Add(time.Second*10).Unix(), item.Start)
	assert.Equal(int64(2), item.Interval)

	item, err = parsePolicyItem(now, []byte("10s:2s:10m"))
	assert.NoError(err)
	assert.Equal(now.Add(time.Second*10).Unix(), item.Start)
	assert.Equal(int64(2), item.Interval)
	assert.Equal(now.Add(time.Minute*10).Unix(), item.End)

	item, err = parsePolicyItem(now, []byte("::10m"))
	assert.Error(err)

	item, err = parsePolicyItem(now, []byte("10m::"))
	assert.Error(err)

	item, err = parsePolicyItem(now, []byte(":::"))
	assert.Error(err)

	item, err = parsePolicyItem(now, []byte(""))
	assert.Error(err)

	item, err = parsePolicyItem(now, []byte("::5m::"))
	assert.Error(err)

	item, err = parsePolicyItem(now, []byte(":xxx:"))
	assert.Error(err)

	item, err = parsePolicyItem(now, []byte("100"))
	assert.Error(err)
}

type NoneJob struct {
	MockJob
}

func (job NoneJob) Do(t time.Time) {}

func TestScheCommand(t *testing.T) {
	assert := test.Assert(t)
	engine := &Engine{}
	now := time.Now()
	p, _ := ParsePolicy(now, "1s")
	scheCommand{&p, now}.execute(engine)
	p2 := p
	scheCommand{&p2, now}.execute(engine)

	p3, _ := ParsePolicy(now, "2s:2s")
	scheCommand{&p3, now}.execute(engine)

	head := engine.queue
	assert.Equal("1s", head.spec)
	assert.Equal("1s", head.brother.spec)
	assert.Equal("2s:2s", head.next.spec)
}

func BenchmarkScheCommand(b *testing.B) {
	debug.SetGCPercent(-1)
	engine := &Engine{}

	now := time.Now()
	specs := [][]byte{[]byte("1s"), []byte("1s:2s"), []byte("2s:3s"), []byte("3s:2s")}
	policies := make([]Policy, len(specs))
	for i := 0; i < len(specs); i++ {
		policies[i], _ = ParsePolicyBytes(now, specs[i])
	}

	unix := now.Add(time.Minute).Unix()

	for i := 0; i < b.N; i++ {
		p := policies[i%len(policies)]
		p.next, p.brother, p.at = nil, nil, 0
		scheCommand{&p, now}.execute(engine)

		for engine.queue != nil && engine.queue.at <= unix {
			iter := engine.queue
			engine.queue = engine.queue.next
			for iter != nil {
				iter = iter.brother
			}
		}
	}
}
