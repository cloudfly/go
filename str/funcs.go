package str

import (
	"bytes"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/cloudfly/go/base36"
	"github.com/cloudfly/go/tools"
)

// Equal 判断 2 个字符串数组是否相同
// 不考虑排序情况，也就是说 [a,b] == [b,a]
func Equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	tableA := make(map[string]struct{})
	tableB := make(map[string]struct{})
	for _, item := range a {
		tableA[item] = struct{}{}
	}
	for _, item := range b {
		tableB[item] = struct{}{}
		if _, ok := tableA[item]; !ok {
			return false
		}
	}
	for _, item := range a {
		if _, ok := tableB[item]; !ok {
			return false
		}
	}
	return true
}

// Distinct 对字符串数组进行去重
func Distinct(arr []string) []string {
	if len(arr) == 0 {
		return arr
	}
	m := make(map[string]struct{}, len(arr))
	for i := range arr {
		m[arr[i]] = struct{}{}
	}
	result := make([]string, 0, len(m))
	for key := range m {
		result = append(result, key)
	}
	return result
}

// Contain check if string s is in the list
func Contain(list []string, s string) bool {
	for _, item := range list {
		if item == s {
			return true
		}
	}
	return false
}

// ContainAny check if list contain any item
func ContainAny(list []string, s ...string) bool {
	set := make(map[string]struct{})
	for _, item := range list {
		set[item] = struct{}{}
	}
	for _, item := range s {
		if _, ok := set[item]; ok {
			return true
		}
	}
	return false
}

// ContainAll check if list contain all items
func ContainAll(list []string, s ...string) bool {
	set := make(map[string]struct{})
	for _, item := range list {
		set[item] = struct{}{}
	}
	for _, item := range s {
		if _, ok := set[item]; !ok {
			return false
		}
	}
	return true
}

func TrimAny(input string, suffixes []string) (string, bool) {
	result := input
	changed := false
	if result == "" {
		return result, false
	}
	for _, s := range suffixes {
		if strings.HasSuffix(result, s) {
			result = strings.TrimSuffix(result, s)
			changed = true
			break
		}
	}
	return result, changed
}

// ParseSearch 解析搜索内容.
// 比如将 key1=value1 key2=value2 some other info 解析成
// {key1:values,key2:values} 和 "some other info" 并返回
func ParseSearch(query string, keys ...string) (map[string]string, string) {
	data := make(map[string]string)
	rest := ""
L:
	for _, item := range strings.Fields(query) {
		i := strings.IndexByte(item, ':')
		if i > 0 && i < len(item)-1 {
			for _, key := range keys {
				if item[:i] == key {
					data[key] = item[i+1:]
					continue L
				}
			}
		}
		if rest == "" {
			rest = item
		} else {
			rest += " " + item
		}
	}
	return data, rest
}

func Split(s string, sep string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(s, sep)
}

func Union(a, b []string) []string {
	tmp := make([]string, 0, len(a)+len(b))
	tmp = append(tmp, a...)
	tmp = append(tmp, b...)
	m := make(map[string]struct{})
	for _, k := range tmp {
		m[k] = struct{}{}
	}
	tmp = tmp[:0]
	for k := range m {
		tmp = append(tmp, k)
	}
	return tmp
}

func And(a, b []string) []string {
	tmp := make([]string, 0, len(a))
	m := make(map[string]struct{})
	for _, k := range a {
		m[k] = struct{}{}
	}
	for _, k := range b {
		if _, ok := m[k]; ok {
			tmp = append(tmp, k)
		}
	}
	return tmp
}

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")

// Rand 返回固定长度的随机字符串
func Rand(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

// Match 模糊匹配
func Match(pattern, s string) bool {
	i, j, star, match := 0, 0, -1, 0
	for i < len(s) {
		if j < len(pattern) && (s[i] == pattern[j] || pattern[j] == '?') {
			i++
			j++
		} else if j < len(pattern) && pattern[j] == '*' {
			match, star = i, j
			j++
		} else if star != -1 {
			j = star + 1
			match++
			i = match
		} else {
			return false
		}
	}
	for ; j < len(pattern); j++ {
		if pattern[j] != '*' {
			return false
		}
	}
	return true
}

// MatchDst 模糊匹配
func MatchDst(pattern, s string, dst []string) ([]string, bool) {
	i, j, star, match, matchStart := 0, 0, -1, 0, -1
	for i < len(s) {
		if j < len(pattern) && (s[i] == pattern[j] || pattern[j] == '?') {
			i++
			j++
		} else if j < len(pattern) && pattern[j] == '*' {
			if matchStart >= 0 && matchStart < match {
				dst = append(dst, s[matchStart:match])
			}
			if match != 0 {
				dst = append(dst, s[match:i])
			}
			matchStart, match, star = i, i, j
			j++
		} else if star != -1 {
			j = star + 1
			match++
			i = match
		} else {
			return dst, false
		}
	}
	for ; j < len(pattern); j++ {
		if pattern[j] != '*' {
			return dst, false
		}
	}
	if matchStart < len(s)-1 {
		dst = append(dst, s[matchStart:])
	}
	return dst, true
}

// CompareVersion 对比 2 个版本昊， v1 > v2 返回 1， v1 < v2 返回 -1, v1 == v2 返回 0
func CompareVersion(v1, v2 string) (int, error) {
	if v1 == v2 {
		return 0, nil
	}
	fields1 := strings.Split(v1, ".")
	fields2 := strings.Split(v2, ".")
	i := 0
	for i < len(fields1) && i < len(fields2) {
		i1, err := strconv.Atoi(fields1[i])
		if err != nil {
			return 0, err
		}
		i2, err := strconv.Atoi(fields2[i])
		if err != nil {
			return 0, err
		}
		switch {
		case i1 == i2:
			break
		case i1 > i2:
			return 1, nil
		case i1 < i2:
			return -1, nil
		}
		i++
	}
	if len(fields1) < len(fields2) {
		return -1, nil
	} else if len(fields1) > len(fields2) {
		return 1, nil
	}
	return 0, fmt.Errorf("different version format")
}

func ReplaceVars(s string, vars map[string]any) string {
	b := &strings.Builder{}
	start := -1
	for i, c := range s {
		if c == '$' {
			start = i
		} else if start != -1 {
			if !varCharTable[c] {
				varName := s[start+1 : i]
				varValue := vars[varName]
				b.WriteString(fmt.Sprintf("%v", varValue))
				b.WriteRune(c)
				start = -1
			}
		} else {
			b.WriteRune(c)
		}
	}
	if start != -1 {
		varName := s[start+1:]
		varValue := vars[varName]
		b.WriteString(fmt.Sprintf("%v", varValue))
	}
	return b.String()
}

func EncodeIdWithKey(prefix string, n uint64, key []byte) string {
	if n <= 0 {
		return ""
	}

	n = tools.FeistelEncrypt(n, key)

	d := []byte(prefix)
	d = append(d, byte(n>>56))
	d = append(d, byte(n>>48))
	d = append(d, byte(n>>40))
	d = append(d, byte(n>>32))
	d = append(d, byte(n>>24))
	d = append(d, byte(n>>16))
	d = append(d, byte(n>>8))
	d = append(d, byte(n))

	return prefix + strings.ToLower(base36.EncodeBytes(d))
}

func DecodeIdWithKey(prefix string, s string, key []byte) (uint64, error) {
	if s == "" {
		return 0, nil
	}
	if !strings.HasPrefix(s, prefix) {
		return 0, fmt.Errorf("invalid id %s", s)
	}
	// trim prefix
	s = strings.TrimPrefix(s, prefix)
	data := base36.DecodeToBytes(s)
	if !bytes.HasPrefix(data, []byte(prefix)) {
		return 0, fmt.Errorf("invalid id %s", s)
	}

	n := uint64(0)
	for i := 0; i < 8; i++ {
		n |= uint64(data[len(data)-1-i]) << (8 * i)
	}

	n = tools.FeistelDecrypt(uint64(n), key)

	return n, nil
}

func EncodeId(prefix string, n uint64) string {
	return EncodeIdWithKey(prefix, n, []byte(strings.Repeat("12345678", 4)))
}

func DecodeId(prefix string, s string) (uint64, error) {
	return DecodeIdWithKey(prefix, s, []byte(strings.Repeat("12345678", 4)))
}

var (
	varChars     = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._"
	varCharTable [128]bool
)

func init() {
	for _, r := range varChars {
		varCharTable[r] = true
	}
}
