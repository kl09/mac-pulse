package collector

import (
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLsof(t *testing.T) {
	t.Parallel()

	// lsof -nP -p <pid> -F ftn of `python3 -m http.server 8799 --bind 127.0.0.1`.
	captured, err := os.ReadFile("testdata/lsof_python.txt")
	require.NoError(t, err)
	var many strings.Builder
	manyFiles := make([]string, maxOpenRows)
	for i := range maxOpenRows + 5 {
		fmt.Fprintf(&many, "f%d\ntREG\nn/tmp/%d\n", i, i)
	}
	for i := range manyFiles {
		manyFiles[i] = fmt.Sprintf("/tmp/%d", i)
	}

	tests := []struct {
		name  string
		input string
		want  ProcDetail
	}{
		{
			name:  "captured: the working directory, the descriptors and the sockets; the 18 loaded libraries are left out",
			input: string(captured),
			want: ProcDetail{
				Files: []string{
					"/Users/alex/src/site", "/dev/null", "/dev/null", "/dev/null",
				},
				Sockets: []string{"IPv4 127.0.0.1:8799", "unix socket"},
			},
		},
		{
			name:  "a listener on every address, a pipe and a kqueue are not files",
			input: "p8817\nf4\ntIPv6\nn*:8799\nf6\ntPIPE\nn->0x1\nf7\ntKQUEUE\nncount=0, state=0\n",
			want:  ProcDetail{Files: []string{}, Sockets: []string{"IPv6 *:8799"}},
		},
		{
			name:  "a name with a bidi control is cleaned, a mapped file is not a descriptor",
			input: "f3\ntREG\nn/tmp/a\u202eb\nfmem\ntREG\nn/tmp/mapped\n",
			want:  ProcDetail{Files: []string{"/tmp/ab"}, Sockets: []string{}},
		},
		{
			name:  "unix sockets known only by a kernel address share one row with their count; a named one keeps its path",
			input: "f3\ntunix\nn->0x9a6910992a79d217\nf4\ntunix\nn/tmp/agent.sock\nf5\ntunix\nn->0x1b\nf6\ntunix\nn->0x2c\n",
			want:  ProcDetail{Files: []string{}, Sockets: []string{"unix socket × 3", "unix /tmp/agent.sock"}},
		},
		{name: "rows past the limit are counted", input: many.String(), want: ProcDetail{Files: manyFiles, Sockets: []string{}, More: 5}},
		{name: "no output", want: ProcDetail{Files: []string{}, Sockets: []string{}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, parseLsof([]byte(tt.input)))
		})
	}
}

func TestReadProcDetail(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pid      int32
		withArgs bool
		wantErr  bool
	}{
		{name: "this process without its command line", pid: int32(os.Getpid())},
		{name: "this process with its command line", pid: int32(os.Getpid()), withArgs: true},
		{name: "launchd belongs to root", pid: 1, withArgs: true, wantErr: true},
		{name: "a pid nothing runs under", pid: math.MaxInt32, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := ReadProcDetail(t.Context(), tt.pid, tt.withArgs)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, ProcDetail{}, got)
				return
			}
			require.NoError(t, err)
			assert.Positive(t, got.Threads)
			assert.NotEmpty(t, got.Files)
			assert.NotNil(t, got.Sockets)
			assert.Equal(t, tt.withArgs, len(got.Args) > 0)
			assert.NotNil(t, got.Args)
		})
	}
}
