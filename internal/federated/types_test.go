package federated

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEncodeU64(t *testing.T) {
	assert.Equal(t, "0", encodeU64(0))
	assert.Equal(t, "1", encodeU64(1))
	assert.Equal(t, "9", encodeU64(9))
	assert.Equal(t, "10", encodeU64(10))
	assert.Equal(t, "999", encodeU64(999))
	assert.Equal(t, "18446744073709551615", encodeU64(18446744073709551615))
}

func TestEncodeI32(t *testing.T) {
	assert.Equal(t, "0", encodeI32(0))
	assert.Equal(t, "5", encodeI32(5))
	assert.Equal(t, "123", encodeI32(123))
	assert.Equal(t, "-1", encodeI32(-1))
	assert.Equal(t, "-20", encodeI32(-20))
	assert.Equal(t, "-9999", encodeI32(-9999))
}

func TestEntryKey(t *testing.T) {
	s := QStateKey{
		CPUBucket:   1,
		MemBucket:   2,
		CPUReqMilli: 500,
		MemReqMilli: 1024,
	}
	a := QActionKey{
		CPUAdjPct: 10,
		MemAdjPct: -5,
	}
	k := EntryKey(s, a)
	assert.Equal(t, "1|2|500|1024|0|0|10|-5", k)
}

func TestEntryKey_AllZeros(t *testing.T) {
	s := QStateKey{}
	a := QActionKey{}
	k := EntryKey(s, a)
	assert.Equal(t, "0|0|0|0|0|0|0|0", k)
}

func TestEntryKey_Uniqueness(t *testing.T) {
	s1 := QStateKey{CPUBucket: 1, MemBucket: 2, CPUReqMilli: 3, MemReqMilli: 4}
	a1 := QActionKey{CPUAdjPct: 5, MemAdjPct: 6}
	s2 := QStateKey{CPUBucket: 1, MemBucket: 2, CPUReqMilli: 3, MemReqMilli: 5}
	a2 := QActionKey{CPUAdjPct: 5, MemAdjPct: 6}
	k1 := EntryKey(s1, a1)
	k2 := EntryKey(s2, a2)
	assert.NotEqual(t, k1, k2, "different MemReqMilli should produce different keys")

	a3 := QActionKey{CPUAdjPct: 6, MemAdjPct: 6}
	k3 := EntryKey(s1, a3)
	assert.NotEqual(t, k1, k3, "different CPUAdjPct should produce different keys")
}
