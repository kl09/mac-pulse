package collector

import (
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/native"
)

func TestWatts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		v       float64
		err     error
		wantNil bool
	}{
		{name: "reading", v: 13.2},
		{name: "zero is a reading: the adapter on battery", v: 0},
		{name: "failed source", v: 13.2, err: native.ErrUnavailable, wantNil: true},
		{name: "any other error", v: 13.2, err: errors.New("boom"), wantNil: true},
		{name: "NaN", v: math.NaN(), wantNil: true},
		{name: "infinity", v: math.Inf(1), wantNil: true},
		{name: "negative", v: -1, wantNil: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := watts(tt.v, tt.err)

			if tt.wantNil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			assert.InDelta(t, tt.v, *got, 0)
		})
	}
}
