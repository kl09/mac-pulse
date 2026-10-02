package native

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFans(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	fans, err := Fans()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("no SMC fans on this machine")
	}
	require.NoError(t, err)

	for _, fan := range fans {
		assert.Less(t, fan.Min, fan.Max)
		assert.GreaterOrEqual(t, fan.RPM, 0.0)
		assert.LessOrEqual(t, fan.RPM, fan.Max*1.1)
	}
}

func TestSystemPower(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	systemW, adapterW, err := SystemPower()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("no SMC power keys on this machine")
	}
	require.NoError(t, err)

	assert.Greater(t, systemW, 0.5)
	assert.Less(t, systemW, 300.0)
	assert.GreaterOrEqual(t, adapterW, 0.0)
	assert.Less(t, adapterW, 300.0)
}

func TestTemps(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("reads this Mac's hardware")
	}

	temps, err := Temps()
	if errors.Is(err, ErrUnavailable) {
		t.Skip("SMC is closed on this machine")
	}
	require.NoError(t, err)
	again, err := Temps()
	require.NoError(t, err)

	assert.GreaterOrEqual(t, len(temps), 10)
	assert.InDelta(t, len(temps), len(again), 20, "the key list is walked once and reused")
	for key, temp := range temps {
		assert.Len(t, key, 4)
		assert.Equal(t, byte('T'), key[0])
		assert.Greater(t, temp, 0.0, key)
		assert.Less(t, temp, 150.0, key)
	}
}

func TestFourCC(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		key  string
		want uint32
	}{
		{name: "fan count", key: "FNum", want: 0x464e756d},
		{name: "key count", key: "#KEY", want: 0x234b4559},
		{name: "type with a trailing space", key: "flt ", want: 0x666c7420},
		{name: "too short", key: "F0", want: 0},
		{name: "too long", key: "F0Acx", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, fourCC(tt.key))
		})
	}
}

func TestFourCCString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code uint32
		want string
	}{
		{name: "fan count", code: 0x464e756d, want: "FNum"},
		{name: "float type", code: 0x666c7420, want: "flt "},
		{name: "zero", code: 0, want: "\x00\x00\x00\x00"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, fourCCString(tt.code))
		})
	}
}

func TestDecodeFloat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  []byte
		want float64
	}{
		{name: "fan minimum 2317 rpm", raw: []byte{0x00, 0xd0, 0x10, 0x45}, want: 2317},
		{name: "stopped fan", raw: []byte{0, 0, 0, 0}, want: 0},
		{name: "temperature 40.0", raw: []byte{0x00, 0x00, 0x20, 0x42}, want: 40},
		{name: "short read", raw: []byte{0x00, 0x20}, want: 0},
		{name: "empty", raw: nil, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.InDelta(t, tt.want, decodeFloat(tt.raw), 0.001)
		})
	}
}
