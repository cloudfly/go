package tools

import "testing"

func TestFeistelDecrypt(t *testing.T) {
	n := uint64(123)
	encrypted := FeistelEncrypt(n, []byte("12345678"))
	decrypted := FeistelDecrypt(encrypted, []byte("12345678"))
	if decrypted != n {
		t.Log(decrypted, "!= ", n)
		t.Fail()
	} else {
		t.Log(n, encrypted, decrypted)
	}
}
