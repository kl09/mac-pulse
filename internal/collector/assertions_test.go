package collector

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseAssertions(t *testing.T) {
	t.Parallel()

	fixture, err := os.ReadFile("testdata/pmset_assertions.txt")
	require.NoError(t, err)

	tests := []struct {
		name  string
		input string
		want  []SleepBlocker
	}{
		{
			name:  "captured fixture keeps caffeinate, drops powerd and UserIsActive",
			input: string(fixture),
			want: []SleepBlocker{
				{PID: 50160, App: "caffeinate", Kind: "PreventUserIdleSystemSleep", Name: "caffeinate command-line tool"},
			},
		},
		{
			name: "legacy assertion, parentheses in the owner and quotes in the reason",
			input: "Listed by owning process:\n" +
				`   pid 714(Google Chrome Helper (Renderer)): [0x0009000000019001] 01:02:03 NoDisplaySleepAssertion named: "Playing "video""  ` + "\n" +
				`   pid 900(coreaudiod): [0x0009000000019002] 00:00:07 PreventUserIdleDisplaySleep named: "com.apple.audio.context"` + "\n" +
				`   pid 901(backupd): [0x0009000000019003] 00:00:07 BackgroundTask named: "com.apple.backupd"` + "\n",
			want: []SleepBlocker{
				{PID: 714, App: "Google Chrome Helper (Renderer)", Kind: "NoDisplaySleepAssertion", Name: `Playing "video"`},
				{PID: 900, App: "coreaudiod", Kind: "PreventUserIdleDisplaySleep", Name: "com.apple.audio.context"},
			},
		},
		{
			name:  "system-wide counters alone are not blockers",
			input: "Assertion status system-wide:\n   PreventUserIdleSystemSleep     1\nListed by owning process:\n",
			want:  []SleepBlocker{},
		},
		{name: "empty input", want: []SleepBlocker{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := parseAssertions(strings.NewReader(tt.input))

			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}
