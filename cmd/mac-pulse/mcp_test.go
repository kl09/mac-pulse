package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kl09/mac-pulse/internal/ui"
)

//nolint:funlen // one table of whole exchanges
func TestServeMCP(t *testing.T) {
	t.Parallel()

	call := func(_ context.Context, name string, args json.RawMessage) (string, error) {
		switch name {
		case "summary":
			return "CPU: 12% " + string(args), nil
		case "history":
			return "", errors.New("open history: no such file")
		}
		return "", fmt.Errorf("%w: unknown tool %q", errMCPParams, name)
	}
	tests := []struct {
		name string
		// in is what the client writes, one request per line; want is what it reads back.
		in   []string
		want []string
	}{
		{
			name: "initialize answers the client's protocol version; a notification gets no answer",
			in: []string{
				`{"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}},"jsonrpc":"2.0","id":0}`,
				`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
				`{"jsonrpc":"2.0","id":"a","method":"ping"}`,
			},
			want: []string{
				`{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2025-06-18",` +
					`"capabilities":{"tools":{}},"serverInfo":{"name":"mac-pulse","version":"dev"}}}`,
				`{"jsonrpc":"2.0","id":"a","result":{}}`,
			},
		},
		{
			name: "initialize without a version names the server's",
			in:   []string{`{"jsonrpc":"2.0","id":1,"method":"initialize"}`},
			want: []string{
				`{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":"2025-11-25",` +
					`"capabilities":{"tools":{}},"serverInfo":{"name":"mac-pulse","version":"dev"}}}`,
			},
		},
		{
			name: "tools/list", in: []string{`{"method":"tools/list","jsonrpc":"2.0","id":1}`},
			want: []string{`{"jsonrpc":"2.0","id":1,"result":{"tools":` + string(mcpTools) + `}}`},
		},
		{
			name: "tools/call passes the arguments and returns the text",
			in:   []string{`{"method":"tools/call","params":{"name":"summary","arguments":{"n":3}},"jsonrpc":"2.0","id":2}`},
			want: []string{`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"CPU: 12% {\"n\":3}"}]}}`},
		},
		{
			name: "a tool that fails answers with isError",
			in:   []string{`{"method":"tools/call","params":{"name":"history"},"jsonrpc":"2.0","id":2}`},
			want: []string{`{"jsonrpc":"2.0","id":2,"result":{"content":[{"type":"text","text":"open history: no such file"}],"isError":true}}`},
		},
		{
			name: "an unknown tool and params of the wrong shape are the client's error",
			in: []string{
				`{"method":"tools/call","params":{"name":"exec"},"jsonrpc":"2.0","id":3}`,
				`{"method":"tools/call","params":[1],"jsonrpc":"2.0","id":4}`,
			},
			want: []string{
				`{"jsonrpc":"2.0","id":3,"error":{"code":-32602,"message":"invalid params: unknown tool \"exec\""}}`,
				`{"jsonrpc":"2.0","id":4,"error":{"code":-32602,"message":"invalid params"}}`,
			},
		},
		{
			name: "a line beyond the limit gets a parse error and the next one still an answer",
			in:   []string{`{"jsonrpc":"2.0","id":7,"m":"` + strings.Repeat("x", mcpMaxLine) + `"}`, `{"jsonrpc":"2.0","id":8,"method":"ping"}`},
			want: []string{
				`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`,
				`{"jsonrpc":"2.0","id":8,"result":{}}`,
			},
		},
		{
			name: "a response of the client's own gets no answer",
			in:   []string{`{"jsonrpc":"2.0","id":11,"result":{}}`, `{"jsonrpc":"2.0","id":12,"method":"ping"}`},
			want: []string{`{"jsonrpc":"2.0","id":12,"result":{}}`},
		},
		{
			name: "an unknown method", in: []string{`{"jsonrpc":"2.0","id":3,"method":"resources/list"}`},
			want: []string{`{"jsonrpc":"2.0","id":3,"error":{"code":-32601,"message":"method not found"}}`},
		},
		{
			name: "a line that is not JSON gets a parse error and the next one still an answer; a blank line is skipped",
			in:   []string{`{"jsonrpc":`, ``, `{"jsonrpc":"2.0","id":5,"method":"ping"}`},
			want: []string{
				`{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"parse error"}}`,
				`{"jsonrpc":"2.0","id":5,"result":{}}`,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			clientOut, serverIn := io.Pipe()
			serverOut, clientIn := io.Pipe()
			served := make(chan error, 1)
			go func() {
				served <- serveMCP(t.Context(), serverOut, serverIn, call)
				_ = serverIn.Close()
			}()
			go func() {
				_, _ = io.WriteString(clientIn, strings.Join(tt.in, "\n")+"\n")
				_ = clientIn.Close()
			}()

			var got []string
			for lines := bufio.NewScanner(clientOut); lines.Scan(); {
				got = append(got, lines.Text())
			}

			require.NoError(t, <-served)
			require.Len(t, got, len(tt.want))
			for i := range tt.want {
				assert.JSONEq(t, tt.want[i], got[i])
			}
		})
	}
}

func TestServeMCP_LastLineWithoutNewline(t *testing.T) {
	t.Parallel()

	var out strings.Builder
	err := serveMCP(t.Context(), strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`), &out, nil)

	require.NoError(t, err)
	assert.JSONEq(t, `{"jsonrpc":"2.0","id":1,"result":{}}`, out.String())
}

// Only the rows that answer before a sample is taken: a sample reads this Mac for two seconds.
func TestCallTool(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	tests := []struct {
		name string
		tool string
		args string
		want string
	}{
		{name: "an unknown tool", tool: "quit_app", want: `invalid params: unknown tool "quit_app"`},
		{name: "arguments of the wrong type get the fixed text, not Go's", tool: "top_apps", args: `{"metric":5}`, want: "invalid params"},
		{name: "arguments that are no object", tool: "summary", args: `[1]`, want: "invalid params"},
		{
			name: "n below the range", tool: "top_apps", args: `{"n":0}`,
			want: "invalid params: top_apps takes metric cpu or memory and n from 1 to 20",
		},
		{
			name: "n above the range", tool: "top_apps", args: `{"n":21}`,
			want: "invalid params: top_apps takes metric cpu or memory and n from 1 to 20",
		},
		{
			name: "a metric outside the list", tool: "top_apps", args: `{"metric":"../../etc/passwd"}`,
			want: "invalid params: top_apps takes metric cpu or memory and n from 1 to 20",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			text, err := callTool(t.Context(), dir, ui.Self{}, tt.tool, json.RawMessage(tt.args))

			require.ErrorIs(t, err, errMCPParams)
			assert.Equal(t, tt.want, err.Error())
			assert.Empty(t, text)
		})
	}
}

func TestTopApps(t *testing.T) {
	t.Parallel()

	items := []ui.App{
		{Name: "idle", CPU: 0, Memory: 300, PIDCount: 1},
		{Name: "busy", CPU: 90, Memory: 100, PIDCount: 4},
		{Name: "tie", CPU: 90, Memory: 200, PIDCount: 2},
	}
	tests := []struct {
		name   string
		metric string
		n      int
		want   string
	}{
		{
			name: "by cpu, equal values in the incoming order", metric: "cpu", n: 2,
			want: `[{"name":"busy","cpu":90,"memory":100,"processes":4},{"name":"tie","cpu":90,"memory":200,"processes":2}]`,
		},
		{name: "by memory", metric: "memory", n: 1, want: `[{"name":"idle","cpu":0,"memory":300,"processes":1}]`},
		{
			name: "n beyond the list gives the whole list", metric: "memory", n: 20,
			want: `[{"name":"idle","cpu":0,"memory":300,"processes":1},{"name":"tie","cpu":90,"memory":200,"processes":2},` +
				`{"name":"busy","cpu":90,"memory":100,"processes":4}]`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := json.Marshal(topApps(slices.Clone(items), tt.metric, tt.n))

			require.NoError(t, err)
			assert.JSONEq(t, tt.want, string(got))
		})
	}
}
