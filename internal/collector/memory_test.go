package collector

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseVMStat(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/vm_stat.txt")
	require.NoError(t, err)
	const page = 16384
	full := Memory{
		App:            (663872 - 27330) * page,
		Wired:          173781 * page,
		Compressed:     266592 * page,
		Cached:         (407796 + 27330) * page,
		Free:           17927 * page,
		PageIns:        88978976 * page,
		PageOuts:       5677387 * page,
		SwapIns:        232772130 * page,
		SwapOuts:       234832696 * page,
		Compressions:   7910064524 * page,
		Decompressions: 7727173386 * page,
	}
	small := Memory{
		App:            (663872 - 27330) * 4096,
		Wired:          173781 * 4096,
		Compressed:     266592 * 4096,
		Cached:         (407796 + 27330) * 4096,
		Free:           17927 * 4096,
		PageIns:        88978976 * 4096,
		PageOuts:       5677387 * 4096,
		SwapIns:        232772130 * 4096,
		SwapOuts:       234832696 * 4096,
		Compressions:   7910064524 * 4096,
		Decompressions: 7727173386 * 4096,
	}
	noApp := full
	noApp.App = 0
	noSwapIns := full
	noSwapIns.SwapIns = 0

	tests := []struct {
		name    string
		input   string
		want    Memory
		wantErr bool
	}{
		{name: "full fixture", input: string(fixture), want: full},
		{
			name:  "page size comes from the header",
			input: strings.Replace(string(fixture), "page size of 16384 bytes", "page size of 4096 bytes", 1),
			want:  small,
		},
		{
			name:  "more purgeable than anonymous pages reads zero app memory",
			input: strings.Replace(string(fixture), "Anonymous pages:                         663872.", "Anonymous pages: 100.", 1),
			want:  noApp,
		},
		{
			name:  "paging counter an older vm_stat lacks reads zero",
			input: strings.Replace(string(fixture), "Swapins:", "Swap-ins:", 1),
			want:  noSwapIns,
		},
		{name: "empty input", input: "", wantErr: true},
		{
			name:    "missing key",
			input:   strings.Replace(string(fixture), "Pages occupied by compressor:", "Pages in compressor:", 1),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseVMStat(strings.NewReader(tt.input))

			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
