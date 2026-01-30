package filterpolicy

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Kinds of policy
const (
	policyKindSingle = iota
	policyKindInterval
	policyKindNever
	policyKindAtEvery
	policyKindAt
)

var (
	rangeRegexp *regexp.Regexp
	// neverTime 代表永不会生效的时间。系统启动时初始化的静态值。
	neverTime time.Time
)

func init() {
	rangeRegexp = regexp.MustCompile("^(.*)-(.*)/(.+)$")
	neverTime = time.Now().AddDate(10, 0, 0).Truncate(time.Second)
}

func Never() time.Time {
	return neverTime
}

// Policy is a real policy executor
type Policy struct {
	spec   string
	items  []*Item
	closed bool
}

// Parse create a new FilterPolicy from a given specification
// eg. 1,2,3,25,100 will pass 1,2,3,25 and 100. others will not
func Parse(spec string) (*Policy, error) {
	policy := &Policy{
		spec:  spec,
		items: make([]*Item, 0, 10),
	}
	fields := strings.Split(spec, ",")
	for _, field := range fields {
		f := strings.TrimSpace(field)
		if f == "" {
			continue
		}
		item, err := NewItem(f)
		if err != nil {
			return nil, fmt.Errorf("unvalid spec '%s': %w", field, err)
		}
		policy.items = append(policy.items, item)
	}
	return policy, nil
}

// Stop ...
func (policy *Policy) Stop() {
	policy.closed = true
}

// Pass 检查当前时间是否满足策略
func (policy *Policy) Pass(start, current time.Time) bool {
	for _, item := range policy.items {
		if item.Pass(start, current) {
			// fmt.Println(start, current, item)
			return true
		}
	}
	return false
}

func (policy *Policy) NextTime(start time.Time, current time.Time) time.Time {
	var t time.Time
	for _, item := range policy.items {
		if next := item.NextTime(start, current); t.IsZero() || t.After(next) {
			// fmt.Println(start, current, item)
			t = next
		}
	}
	return t
}

// PassedBefore 判断在 current 时间之前，是否存在一个时间点满足策略
func (policy *Policy) PassedBefore(start, current time.Time) bool {
	for _, item := range policy.items {
		if item.PassedBefore(start, current) {
			return true
		}
	}
	return false
}

// Item is a policy item, a part of Item
type Item struct {
	spec  string
	kind  int
	min   time.Duration
	max   time.Duration
	value time.Duration
}

// NewItem create a new policy item from a specification
// <integer>: represents exact match
// */<integer>: represents the number should be divisible by <ingeger>
// <max-integer>-<max-ingeter>: represents a number range, only numbers in this range can be passed
// eg. 34 or */3 or 3-12
func NewItem(spec string) (*Item, error) {
	if spec == "" {
		return nil, fmt.Errorf("empty specification")
	}

	if spec == "-" {
		return &Item{
			spec: spec,
			kind: policyKindNever,
		}, nil
	}

	if strings.HasPrefix(spec, "*/") && len(spec) >= 3 {
		// 等同于 "-/" 开头，为了向后兼容，没有去掉
		interval := spec[2:]
		dur, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %s: %w", interval, err)
		}
		return &Item{
			spec:  spec,
			kind:  policyKindInterval,
			value: dur,
		}, nil
	} else if strings.HasPrefix(spec, "@/") && len(spec) >= 4 {
		// 比如 @/1h，@/1m 代表整点时间。
		interval := spec[2:]
		dur, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %s: %w", interval, err)
		}
		return &Item{
			spec:  spec,
			kind:  policyKindAtEvery,
			value: dur,
		}, nil
	} else if strings.HasPrefix(spec, "@") && len(spec) > 3 {
		// 比如 @1h，@1m 代表整点时间。
		interval := spec[1:]
		dur, err := time.ParseDuration(interval)
		if err != nil {
			return nil, fmt.Errorf("invalid duration %s: %w", interval, err)
		}
		return &Item{
			spec:  spec,
			kind:  policyKindAt,
			value: dur,
		}, nil
	} else if rangeRegexp.MatchString(spec) {
		submatchs := rangeRegexp.FindStringSubmatch(spec)
		var (
			min, max, interval time.Duration
			err                error
		)
		if submatchs[1] != "" {
			min, err = time.ParseDuration(submatchs[1])
			if err != nil {
				return nil, fmt.Errorf("invalid duration '%s' in '%s'", submatchs[1], spec)
			}
		}
		if submatchs[2] != "" {
			max, err = time.ParseDuration(submatchs[2])
			if err != nil {
				return nil, fmt.Errorf("invalid duration '%s' in '%s'", submatchs[2], spec)
			}
		}
		if submatchs[3] != "" {
			interval, err = time.ParseDuration(submatchs[3])
			if err != nil {
				return nil, fmt.Errorf("invalid duration '%s' in '%s'", submatchs[3], spec)
			}
		}
		if interval == 0 {
			return nil, fmt.Errorf("zero interval duration '%s'in '%s'", interval, spec)
		}
		if max > 0 && max-min < interval {
			return nil, fmt.Errorf("invalid range duration '%s'", spec)
		}
		return &Item{
			spec:  spec,
			kind:  policyKindInterval,
			min:   min,
			max:   max,
			value: interval,
		}, nil
	}

	dur, err := time.ParseDuration(spec)
	if err != nil {
		return nil, fmt.Errorf("unvalid integer %s: %w", spec, err)
	}
	return &Item{
		spec:  spec,
		kind:  policyKindSingle,
		value: dur,
	}, nil
}

func (item *Item) Pass(start, current time.Time) bool {
	// 当前时间还未达到 start time，表示未发生呢
	if current.Before(start) {
		return false
	}
	switch item.kind {
	case policyKindNever:
		return false
	case policyKindAt:
		return start.Truncate(item.value).Add(item.value).Equal(current)
	case policyKindAtEvery:
		return current.Unix()%int64(item.value/time.Second) == 0
	case policyKindSingle:
		return start.Add(item.value).Equal(current)
	case policyKindInterval:
		sub := current.Sub(start)
		if item.min != 0 && sub < item.min {
			return false
		}
		if item.max != 0 && sub > item.max {
			return false
		}
		if current.Equal(start) {
			return false
		}
		return current.Sub(start)%item.value == 0
	}
	return false
}

func (item *Item) NextTime(start time.Time, current time.Time) time.Time {
	switch item.kind {
	case policyKindNever:
		return neverTime
	case policyKindAt:
		next := start.Truncate(item.value).Add(item.value)
		if next.After(current) {
			return next
		}
		return neverTime
	case policyKindAtEvery:
		return current.Truncate(item.value).Add(item.value)
	case policyKindSingle:
		next := start.Add(item.value)
		if next.Before(current) {
			return neverTime
		}
		return next
	case policyKindInterval:
		next := current.Truncate(item.value).Add(item.value)
		if item.max != 0 && next.After(start.Add(item.max)) {
			return neverTime
		}
		if item.min != 0 && next.Before(start.Add(item.min)) {
			return start.Add(item.min).Truncate(item.value).Add(item.value)
		}
		return next
	}
	return neverTime
}

func (item *Item) PassedBefore(start, current time.Time) bool {
	switch item.kind {
	case policyKindNever:
		return false
	case policyKindAtEvery:
		return start.Truncate(item.value).Add(item.value).Before(current)
	case policyKindSingle:
		return start.Add(item.value).Before(current)
	case policyKindInterval:
		sub := current.Sub(start)
		if item.min != 0 && sub < item.min {
			return false
		}
		return start.Add(item.value).Before(current)
	}
	return false
}
