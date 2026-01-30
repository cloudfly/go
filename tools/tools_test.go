package tools

import (
	"testing"

	"github.com/cloudfly/go/test"
)

func TestHash(t *testing.T) {
	assert := test.Assert(t)
	h1 := Hash([]byte("hello"))
	h2 := Hash([]byte("hello"))
	assert.Equal(h1, h2)
	h3 := Hash([]byte("hello "))
	assert.NotEqual(h1, h3)
}

func TestSimpleMatch(t *testing.T) {
	assert := test.Assert(t)
	assert.True(SimpleMatch("*", "abc"))
	assert.True(SimpleMatch("*hello", "hello"))
	assert.True(SimpleMatch("*hello", "gogohello"))
	assert.True(SimpleMatch("*hello*", "gogohello world"))
	assert.True(SimpleMatch("**", "gogohello world"))
	assert.False(SimpleMatch("*p*", "world"))
	assert.True(SimpleMatch("*", ""))
	assert.True(SimpleMatch("", ""))
	assert.False(SimpleMatch("", "abc"))
	assert.False(SimpleMatch("abc", ""))
	assert.True(SimpleMatch("abc", "abc"))
	assert.False(SimpleMatch("abc", "abcd"))
	assert.False(SimpleMatch("abcd", "ab"))

	assert.True(SimpleMatch("he*wor*", "hello world"))
	assert.True(SimpleMatch("he*wor*d", "hello world"))
	assert.False(SimpleMatch("he*you", "hello world"))
}
