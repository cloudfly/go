package tools

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

// Feistel网络解密（还原int64）
func FeistelDecrypt(data uint64, key []byte) uint64 {
	l, r := uint32(data>>32), uint32(data)
	roundKeys := generateRoundKeys(key, 16)

	for i := 15; i >= 0; i-- { // 反向使用轮密钥
		newL := r
		newR := l ^ f(r, roundKeys[i])
		l, r = newL, newR
	}
	return uint64(r)<<32 | uint64(l)
}

// 轮函数：基于HMAC-SHA256的非线性变换
func f(r uint32, roundKey []byte) uint32 {
	rBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(rBytes, r)

	h := hmac.New(sha256.New, roundKey)
	h.Write(rBytes)
	sum := h.Sum(nil)

	return binary.BigEndian.Uint32(sum[:4]) // 取前32位
}

// Feistel网络加密（混淆int64）
func FeistelEncrypt(data uint64, key []byte) uint64 {
	l, r := uint32(data>>32), uint32(data)
	roundKeys := generateRoundKeys(key, 16) // 16轮变换

	for i := 0; i < 16; i++ {
		newL := r
		newR := l ^ f(r, roundKeys[i])
		l, r = newL, newR
	}
	return uint64(r)<<32 | uint64(l) // 最后一轮交换
}

// 生成轮密钥（16轮，每轮32位）
func generateRoundKeys(mainKey []byte, rounds int) [][]byte {
	keys := make([][]byte, rounds)
	for i := 0; i < rounds; i++ {
		h := hmac.New(sha256.New, mainKey)
		h.Write([]byte{byte(i)}) // 用轮次区分密钥
		sum := h.Sum(nil)
		keys[i] = sum[:4] // 每轮密钥32位
	}
	return keys
}

type DecodeFunc func(any) error
type HandleFunc func(context.Context, DecodeFunc) (any, error)
type RegisterFunc func(string, string, HandleFunc)

type ServiceMethodSpec struct {
	Name     string
	Options  map[string]string
	Request  string
	Response string
}
