package tools

import (
	"crypto/md5"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
)

func Hash(data []byte) string {
	s := sha1.Sum(data)
	m := md5.Sum(data)
	return fmt.Sprintf("%x%x", s[:8], m[:8])
}

func SimpleMatch(pattern, s string) bool {
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


func MapMatch(pattern map[string]string, data map[string]string) int {
	if len(pattern) > len(data) { // pattern 必定存在某个 key, 在 data 中是找不到的
		return 0
	}

	score := 1
	for key, pattern := range pattern {
		value, ok := data[key]
		if !ok {
			return 0
		}
		if i := SimpleMatchScore(pattern, value); i > 0 {
			score += i
		} else {
			return 0
		}
	}
	return score
}

// 字符串匹配, 返回匹配的分值
// - 精准匹配: 2分
// - 模糊匹配: 1分
// - 不匹配: 0分
func SimpleMatchScore(pattern, s string) int {
	if pattern == s { // empty pattern only match empty string
		return 2
	}
	if SimpleMatch(pattern, s) {
		return 1
	}
	return 0
}

func RSAPublickDecryptEasy(key, data []byte) ([]byte, error) {
	block, _ := pem.Decode(key)
	if block == nil || block.Type != "PUBLIC KEY" {
		return nil, errors.New("failed to decode PEM block containing public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("failed to parse public key: %s", err.Error())
	}
	return RSAPublickDecrypt(pub.(*rsa.PublicKey), data), nil
}

func RSAPublickDecrypt(pubKey *rsa.PublicKey, data []byte) []byte {
	c := new(big.Int)
	m := new(big.Int)
	m.SetBytes(data)
	e := big.NewInt(int64(pubKey.E))
	c.Exp(m, e, pubKey.N)
	out := c.Bytes()
	skip := 0
	for i := 2; i < len(out); i++ {
		if i+1 >= len(out) {
			break
		}
		if out[i] == 0xff && out[i+1] == 0 {
			skip = i + 2
			break
		}
	}
	return out[skip:]
}
