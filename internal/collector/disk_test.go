package collector

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSMART(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/diskutil_info.plist")
	require.NoError(t, err)

	tests := []struct {
		name      string
		input     string
		wantModel string
		wantSMART string
		wantErr   bool
	}{
		{name: "full fixture", input: string(fixture), wantModel: "APPLE SSD AP0512Z", wantSMART: SMARTVerified},
		{
			name:      "failing disk",
			input:     strings.Replace(string(fixture), "<string>Verified</string>", "<string>Failing</string>", 1),
			wantModel: "APPLE SSD AP0512Z", wantSMART: SMARTFailing,
		},
		{
			name:      "enclosure that passes no status through",
			input:     strings.Replace(string(fixture), "<string>Verified</string>", "<string>Not Supported</string>", 1),
			wantModel: "APPLE SSD AP0512Z",
		},
		{
			name:      "no status key",
			input:     strings.Replace(string(fixture), "<key>SMARTStatus</key>", "<key>Other</key>", 1),
			wantModel: "APPLE SSD AP0512Z",
		},
		{
			name:      "model with an XML entity",
			input:     strings.Replace(string(fixture), "APPLE SSD AP0512Z</string>", "Fast &amp; Small</string>", 1),
			wantModel: "Fast & Small", wantSMART: SMARTVerified,
		},
		{name: "empty input", input: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			model, smart, err := parseSMART(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantModel, model)
			assert.Equal(t, tt.wantSMART, smart)
		})
	}
}

func TestEjectable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		mount string
		want  bool
	}{
		{mount: "/"},
		{mount: "/Volumes/Backup", want: true},
		{mount: "/Volumes/My Disk/nested", want: true},
		{mount: "/System/Volumes/Data"},
		{mount: "/System/Volumes/Data/home"},
		{mount: "/System/Volumes/VM"},
		{mount: "/dev"},
		{mount: "/Volumes"},
		{mount: ""},
	}
	for _, tt := range tests {
		t.Run(tt.mount, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, ejectable(tt.mount))
		})
	}
}

func TestEject(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mount   string
		wantErr string
	}{
		{name: "the boot volume is refused before diskutil runs", mount: "/", wantErr: `eject "/": not a volume under /Volumes`},
		{name: "a path that climbs out of /Volumes is refused", mount: "/Volumes/../", wantErr: "not a volume under /Volumes"},
		{name: "a path outside /Volumes is refused", mount: "/System/Volumes/Data", wantErr: "not a volume under /Volumes"},
		{
			name: "a volume that is not mounted fails in diskutil's own words", mount: "/Volumes/mac-pulse no such volume",
			wantErr: "Failed to find disk /Volumes/mac-pulse no such volume",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Eject(t.Context(), tt.mount)

			require.ErrorContains(t, err, tt.wantErr)
		})
	}
}
