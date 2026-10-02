package ui

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The mock is the frontend's only view of the contract while it is designed in a browser,
// so it must round-trip through the Go types without losing or gaining a key.
func TestMock_RoundTrip(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("web/mock.js")
	require.NoError(t, err)
	start, end := bytes.IndexByte(raw, '{'), bytes.LastIndexByte(raw, '}')
	require.Positive(t, start)
	var mock map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw[start:end+1], &mock))

	tests := []struct {
		key    string
		target any
	}{
		{key: "state", target: &State{}},
		{key: "history", target: &History{}},
		{key: "notice", target: &Notice{}},
		{key: "alert_details", target: &[]AlertDetail{}},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			t.Parallel()

			dec := json.NewDecoder(bytes.NewReader(mock[tt.key]))
			dec.DisallowUnknownFields()
			require.NoError(t, dec.Decode(tt.target))
			got, err := json.Marshal(tt.target)
			require.NoError(t, err)

			assert.JSONEq(t, string(mock[tt.key]), string(got))
		})
	}
}
