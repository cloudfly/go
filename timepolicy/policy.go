package timepolicy

import (
	"context"
	"fmt"
	"time"
)

const (
	policySplit     = ','
	policyItemSplit = ':'
)

// Policy represent a group of policy item
type Policy struct {
	spec    string
	items   []policyItem
	job     Job
	at      int64
	brother *Policy
	next    *Policy
}

// ParsePolicy 解析策略字符串
func ParsePolicy(from time.Time, s string) (Policy, error) {
	return ParsePolicyBytes(from, []byte(s))
}

// ParsePolicyBytes 解析策略数组
func ParsePolicyBytes(from time.Time, s []byte) (Policy, error) {
	policy := Policy{
		spec:  string(s),
		items: make([]policyItem, 0, 4),
	}
	start := 0
	for i := 0; i <= len(s); i++ {
		if (i == len(s) || s[i] == policySplit) && i > start {
			item, err := parsePolicyItem(from, s[start:i])
			if err != nil {
				return Policy{}, err
			}
			policy.items = append(policy.items, item)
			start = i + 1
		}
	}
	return policy, nil
}

// NextTime 返回下一次执行策略的时间, 基于参数 now 计算
func (policy *Policy) NextTime(now int64) int64 {
	var latest int64
	for _, item := range policy.items {
		next := item.next(now)
		if next != 0 && (next < latest || latest == 0) {
			latest = next
		}
	}
	return latest
}

type policyItem struct {
	Start    int64 // unix seconds
	Interval int64 // seconds
	End      int64 // unix seconds
}

func parsePolicyItem(from time.Time, s []byte) (policyItem, error) {
	var (
		start     = 0
		durations [3]time.Duration
		index     int
	)
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == policyItemSplit {
			if i == start {
				durations[index] = 0
			} else {
				dur, err := time.ParseDuration(string(s[start:i]))
				if err != nil {
					return policyItem{}, fmt.Errorf("uncorrect interval setting '%s': %s", s[start:i], err.Error())
				}
				durations[index] = dur
			}
			index++
			if index == 3 && i != len(s) { // 已经解析出 3 个 duration 了但是字符串还没有遍历结束
				return policyItem{}, fmt.Errorf("uncorrect policy '%s'", s)
			}
			start = i + 1
		}
	}

	item := policyItem{}

	switch index { // 此时 index 即为 duration 的个数
	case 1:
		item.Interval = int64(durations[0] / time.Second)
	case 2:
		if durations[0] > 0 {
			item.Start = from.Add(durations[0]).Unix()
		}
		item.Interval = int64(durations[1] / time.Second)
	case 3:
		if durations[0] > 0 {
			item.Start = from.Add(durations[0]).Unix()
		}
		item.Interval = int64(durations[1] / time.Second)
		if durations[2] > 0 {
			item.End = from.Add(durations[2]).Unix()
		}

		if item.Start != 0 && item.End != 0 && item.Start > item.End {
			return policyItem{}, fmt.Errorf("the start time later than end time")
		}

	default:
		return policyItem{}, fmt.Errorf("uncorrect policy '%s'", s)
	}

	if item.Interval <= 0 {
		return policyItem{}, fmt.Errorf("uncorrect interval setting, should longger(or equal) than 1s")
	}

	return item, nil
}

func (item policyItem) next(now int64) int64 {
	if item.End != 0 && now > item.End { // 已经结束
		return 0
	}

	if item.Start != 0 && now < item.Start {
		return item.Start
	}
	var mod int64
	if item.Start == 0 {
		mod = now % item.Interval
	} else {
		mod = (now - item.Start) % item.Interval
	}
	if mod == 0 {
		return now
	}
	next := now + item.Interval - mod
	if item.End > 0 && next > item.End {
		return 0
	}
	return next
}

// Job represents the job type which the engine will call it by go Func()
type Job interface {
	Do(time.Time)
	Finished() bool
}

// Engine will run process the policy and run the function at a right time
type Engine struct {
	ctx   context.Context
	ch    chan engineCommand
	queue *Policy
}

// NewEngine create a engine
func NewEngine(ctx context.Context) *Engine {
	engine := Engine{
		ctx: ctx,
		ch:  make(chan engineCommand, 10),
	}
	go engine.activate()
	return &engine
}

// RegisterWithTime a policy to engine
func (engine *Engine) RegisterWithTime(from time.Time, policy string, job Job) error {
	if job == nil {
		return nil
	}
	p, err := ParsePolicy(from, policy)
	if err != nil {
		return err
	}
	p.job = job
	engine.ch <- scheCommand{&p, time.Now()}
	return nil
}

// Register a policy to engine with using time.Now() as from time
func (engine *Engine) Register(policy string, job Job) error {
	return engine.RegisterWithTime(time.Now(), policy, job)
}

// Clear all the jobs
func (engine *Engine) Clear() {
	engine.ch <- clearCommand{}
}

func (engine *Engine) activate() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	done := engine.ctx.Done()

	for {
		select {
		case cmd := <-engine.ch:
			cmd.execute(engine)
		case now := <-ticker.C:
			unix := now.Unix()
			for engine.queue != nil && engine.queue.at <= unix {
				iter := engine.queue
				engine.queue = engine.queue.next
				for iter != nil {
					if !iter.job.Finished() {
						go iter.job.Do(time.Unix(iter.at, 0))
						engine.ch <- scheCommand{iter, time.Unix(iter.at+1, 0)}
					}
					iter = iter.brother
				}
			}
		case <-done:
			return
		}
	}
}

type engineCommand interface {
	execute(*Engine)
}

type scheCommand struct {
	p    *Policy
	from time.Time
}

func (cmd scheCommand) execute(engine *Engine) {
	cmd.p.at = cmd.p.NextTime(cmd.from.Unix())
	if engine.queue == nil {
		cmd.p.next = nil
		cmd.p.brother = nil
		engine.queue = cmd.p
		return
	}
	var prev *Policy
	iter := engine.queue
	for iter != nil {
		if iter.at == cmd.p.at {
			cmd.p.next = iter.next
			cmd.p.brother = iter
			iter.next = nil
			if prev != nil {
				prev.next = cmd.p
			} else {
				engine.queue = cmd.p
			}
			return
		} else if iter.at > cmd.p.at {
			cmd.p.next = iter
			if prev != nil {
				prev.next = cmd.p
			} else {
				engine.queue = cmd.p
			}
			return
		}
		prev = iter
		iter = iter.next
	} // END FOR

	// 加到最末尾
	prev.next = cmd.p
}

type clearCommand struct{}

func (cmd clearCommand) execute(engine *Engine) {
	engine.queue = nil
}
