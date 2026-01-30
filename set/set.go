package set

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sync"
)

// EmptySet 为一个固定的空 Set 结构, 经常用来复用, 避免创建太多的空 Set
var (
	Empty Set[int]
)

type Value interface {
	int | string | int64 | int32 | int16 | int8 | uint32 | uint64 | uint16 | uint8 | float64 | float32
}

func init() {
	Empty.lock = &sync.RWMutex{}
}

// Set 结构存储不重复的字符串数组
type Set[T Value] struct {
	lock *sync.RWMutex
	data map[T]struct{}
}

// NewSet 创建一个新的 Set 结构并返回
func New[T Value](args ...T) Set[T] {
	set := Set[T]{
		lock: &sync.RWMutex{},
		data: make(map[T]struct{}),
	}
	for _, item := range args {
		item := item
		set.data[item] = struct{}{}
	}
	return set
}

// IsZero 判断 Set 结构是不是 Zero Value
func (set *Set[T]) IsZero() bool {
	return set.lock == nil && set.data == nil
}

// IsEmpty 判断 Set 结构是不是 Zero Value
func (set *Set[T]) IsEmpty() bool {
	if set.IsZero() {
		return true
	}
	return set.Len() == 0
}

// Scan 将 mysql 查询出的结果存储变量 set 中
func (set *Set[T]) Scan(src any) error {
	if src == nil {
		return nil
	}
	var content []byte
	switch value := src.(type) {
	case string:
		content = []byte(value)
	case []byte:
		content = value
	default:
		return fmt.Errorf("can not convert %#v into Set", src)
	}
	content2 := make([]byte, len(content)+2)
	content2[0] = '['
	copy(content2[1:len(content2)-1], content)
	content2[len(content2)-1] = ']'
	var data []T
	if err := json.Unmarshal(content2, &data); err != nil {
		return err
	}

	*set = New(data...)
	return nil
}

// Value 实现 driver.Valuer,使得该结构可用于 mysql 的存储
func (set Set[T]) Value() (driver.Value, error) {
	return set.String(), nil
}

// Has 检验 Set 是否包含某值
func (set Set[T]) Has(value T) bool {
	set.lock.RLock()
	defer set.lock.RUnlock()
	if set.data == nil {
		return false
	}
	_, ok := set.data[value]
	return ok
}

// Insert 向 Set 中添加元素
func (set Set[T]) Insert(value T) {
	set.lock.Lock()
	defer set.lock.Unlock()
	if set.data == nil {
		set.data = make(map[T]struct{})
	}
	set.data[value] = struct{}{}
}

// Clear 清空 Set 中元素
func (set Set[T]) Clear() {
	set.lock.Lock()
	defer set.lock.Unlock()
	for k := range set.data {
		delete(set.data, k)
	}
}

// Del 从 Set 中删除元素
func (set Set[T]) Del(value T) {
	set.lock.Lock()
	defer set.lock.Unlock()
	if set.data == nil {
		return
	}
	delete(set.data, value)
}

func (set Set[T]) String() string {
	if len(set.data) == 0 {
		return ""
	}
	content, _ := json.Marshal(set.Slice())
	return string(content[1 : len(content)-1])
}

// Slice 返回 Set 的切片结构
func (set Set[T]) Slice() []T {
	if len(set.data) == 0 {
		return []T{}
	}
	set.lock.RLock()
	defer set.lock.RUnlock()
	tmp := make([]T, 0, len(set.data))
	for value := range set.data {
		tmp = append(tmp, value)
	}
	return tmp
}

// Add 将两个 set 合并并返回新 set, 不改变原数据
func (set Set[T]) Add(other Set[T]) Set[T] {
	result := New[T]()
	set.lock.RLock()
	defer set.lock.RUnlock()
	other.lock.RLock()
	defer other.lock.RUnlock()
	for value := range other.data {
		result.data[value] = struct{}{}
	}
	for value := range set.data {
		result.data[value] = struct{}{}
	}
	return result
}

// Sub 求 set - other 并返回
func (set Set[T]) Sub(other Set[T]) Set[T] {
	result := New[T]()
	set.lock.RLock()
	defer set.lock.RUnlock()
	other.lock.RLock()
	defer other.lock.RUnlock()
	for value := range set.data {
		if _, ok := other.data[value]; !ok {
			result.data[value] = struct{}{}
		}
	}
	return result
}

// Union 求两个 set 集合的并集
func (set Set[T]) Union(other Set[T]) Set[T] {
	result := New[T]()
	set.lock.RLock()
	defer set.lock.RUnlock()
	other.lock.RLock()
	defer other.lock.RUnlock()
	for value := range set.data {
		if _, ok := other.data[value]; ok {
			result.data[value] = struct{}{}
		}
	}
	return result
}

// Len 返回元素的个数
func (set Set[T]) Len() int {
	set.lock.RLock()
	defer set.lock.RUnlock()
	return len(set.data)
}

// MarshalJSON encode status value into json
func (set Set[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal(set.Slice())
}

// UnmarshalJSON decode status value from json
func (set *Set[T]) UnmarshalJSON(data []byte) error {
	if data == nil {
		*set = New[T]()
		return nil
	}
	var arr []T
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	*set = New(arr...)
	return nil
}
