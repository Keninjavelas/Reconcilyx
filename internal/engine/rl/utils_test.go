package rl

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseInt64(t *testing.T) {
	tests := []struct {
		input    string
		expected int64
		ok       bool
	}{
		{"0", 0, true},
		{"1", 1, true},
		{"42", 42, true},
		{"1234567890", 1234567890, true},
		{"-1", -1, true},
		{"-999", -999, true},
		{"", 0, false},
		{"-", 0, false},
		{"abc", 0, false},
		{"12a3", 0, false},
		{"--1", 0, false},
		{"1.5", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, ok := parseInt64(tt.input)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.expected, v)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	v, ok := parseInt("123")
	assert.True(t, ok)
	assert.Equal(t, 123, v)

	v, ok = parseInt("-45")
	assert.True(t, ok)
	assert.Equal(t, -45, v)

	_, ok = parseInt("xyz")
	assert.False(t, ok)

	_, ok = parseInt("")
	assert.False(t, ok)
}

func TestSplitN(t *testing.T) {
	assert.Nil(t, splitN("a|b|c", "|", 0))

	assert.Equal(t, []string{"a"}, splitN("a", "|", 1))
	assert.Equal(t, []string{"a|b|c"}, splitN("a|b|c", "|", 1))

	assert.Equal(t, []string{"a", "b"}, splitN("a|b", "|", 2))
	assert.Equal(t, []string{"a", "b|c"}, splitN("a|b|c", "|", 2))

	assert.Equal(t, []string{"1", "2", "3", "4", "5", "6"}, splitN("1|2|3|4|5|6", "|", 6))
	assert.Equal(t, []string{"a", "b", "c", ""}, splitN("a|b|c|", "|", 4))
	assert.Equal(t, []string{"", "a", "b"}, splitN("|a|b", "|", 3))
}

func TestParseEntryKey_Legacy6Fields(t *testing.T) {
	k, ok := parseEntryKey("1|2|500|1024|10|-5")
	assert.True(t, ok)
	assert.Equal(t, int64(1), k.S.CPUBucket)
	assert.Equal(t, int64(2), k.S.MemBucket)
	assert.Equal(t, int64(500), k.S.CPUReqMilli)
	assert.Equal(t, int64(1024), k.S.MemReqMilli)
	assert.Equal(t, int64(0), k.S.JVMBucket)
	assert.Equal(t, int64(0), k.S.GPUBucket)
	assert.Equal(t, 10, k.A.CPUAdjPct)
	assert.Equal(t, -5, k.A.MemAdjPct)
}

func TestParseEntryKey_New8Fields(t *testing.T) {
	k, ok := parseEntryKey("1|2|500|1024|2|3|10|-5")
	assert.True(t, ok)
	assert.Equal(t, int64(1), k.S.CPUBucket)
	assert.Equal(t, int64(2), k.S.MemBucket)
	assert.Equal(t, int64(500), k.S.CPUReqMilli)
	assert.Equal(t, int64(1024), k.S.MemReqMilli)
	assert.Equal(t, int64(2), k.S.JVMBucket)
	assert.Equal(t, int64(3), k.S.GPUBucket)
	assert.Equal(t, 10, k.A.CPUAdjPct)
	assert.Equal(t, -5, k.A.MemAdjPct)
}

func TestParseEntryKey_New8FieldsWithZeroBuckets(t *testing.T) {
	k, ok := parseEntryKey("5|3|100|2048|0|0|-20|20")
	assert.True(t, ok)
	assert.Equal(t, int64(5), k.S.CPUBucket)
	assert.Equal(t, int64(3), k.S.MemBucket)
	assert.Equal(t, int64(100), k.S.CPUReqMilli)
	assert.Equal(t, int64(2048), k.S.MemReqMilli)
	assert.Equal(t, int64(0), k.S.JVMBucket)
	assert.Equal(t, int64(0), k.S.GPUBucket)
	assert.Equal(t, -20, k.A.CPUAdjPct)
	assert.Equal(t, 20, k.A.MemAdjPct)
}

func TestParseEntryKey_Invalid(t *testing.T) {
	invalidCases := []string{
		"",
		"1|2|3",
		"1|2|3|4|5",
		"1|2|3|4|5|6|7",
		"a|2|3|4|5|6",
		"1|b|3|4|5|6",
		"1|2|c|4|5|6",
		"1|2|3|d|5|6",
		"1|2|3|4|e|6",
		"1|2|3|4|5|f",
		"1|2|3|4|j|3|10|-5",
		"1|2|3|4|2|g|10|-5",
	}
	for _, s := range invalidCases {
		t.Run(s, func(t *testing.T) {
			_, ok := parseEntryKey(s)
			assert.False(t, ok, "expected invalid for %q", s)
		})
	}
}

func TestIsFinite(t *testing.T) {
	assert.True(t, isFinite(0))
	assert.True(t, isFinite(1.0))
	assert.True(t, isFinite(-99.5))
	assert.True(t, isFinite(1e-300))
	assert.True(t, isFinite(1e15))
	assert.True(t, isFinite(-1e15))
}

func TestSigmoid01(t *testing.T) {
	assert.InDelta(t, 0.5, sigmoid01(0), 1e-9)
	assert.InDelta(t, 0.7310, sigmoid01(1), 0.001)
	assert.InDelta(t, 0.2689, sigmoid01(-1), 0.001)
	assert.Greater(t, sigmoid01(1000), 0.999)
	assert.Less(t, sigmoid01(-1000), 0.001)

	prev := -1.0
	for _, x := range []float64{-10, -5, -1, 0, 1, 5, 10} {
		v := sigmoid01(x)
		assert.Greater(t, v, prev, "sigmoid should be monotonically increasing")
		assert.Greater(t, v, 0.0)
		assert.Less(t, v, 1.0)
		prev = v
	}
}
