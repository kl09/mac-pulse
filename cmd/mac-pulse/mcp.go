package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"

	"github.com/kl09/mac-pulse/internal/collector"
	"github.com/kl09/mac-pulse/internal/store"
	"github.com/kl09/mac-pulse/internal/ui"
)

const (
	// The version answered to a client that names none; otherwise the client's own comes back.
	mcpProtocol = "2025-11-25"
	// A request is one line; no client has reason to send a longer one.
	mcpMaxLine    = 1 << 20
	mcpTopApps    = 5
	mcpTopAppsMax = 20

	mcpParseError     = -32700
	mcpMethodNotFound = -32601
	mcpInvalidParams  = -32602
)

var (
	errMCPParams = errors.New("invalid params")
	errMCPLine   = errors.New("request line too long")
)

// mcpTools is the answer to tools/list.
var mcpTools = json.RawMessage(`[
{"name":"summary","inputSchema":{"type":"object","properties":{}},
 "description":"Current CPU, memory, disk, network, battery, temperature and power of this Mac, with the busiest apps, as plain text."},
{"name":"top_apps",
 "inputSchema":{"type":"object","properties":{
  "metric":{"type":"string","enum":["cpu","memory"]},
  "n":{"type":"integer","minimum":1,"maximum":20}}},
 "description":"The apps using the most CPU or memory now, as JSON: name, cpu (percent of one core), memory (bytes), processes."},
{"name":"history",
 "inputSchema":{"type":"object","required":["metric","range"],"properties":{
  "metric":{"type":"string","enum":["cpu","memory","network","disk","gpu","temp","battery","power"]},
  "range":{"type":"string","enum":["1h","12h","24h","7d","30d","90d","1y"]}}},
 "description":"History of one metric, as JSON: points step_s apart from start (unix ms), null where mac-pulse was not running."}
]`)

type mcpRequest struct {
	// ID is absent on a notification, which gets no answer.
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// runMCP is the `mac-pulse mcp` subcommand: a Model Context Protocol server on stdin and
// stdout that lives until stdin closes. It opens no socket and takes no instance lock.
func runMCP(ctx context.Context) error {
	base, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate Application Support: %w", err)
	}
	dir := filepath.Join(base, "mac-pulse")
	self := ui.Self{UID: uint32(os.Getuid()), PID: int32(os.Getpid())}
	served := make(chan error, 1)
	go func() {
		served <- serveMCP(ctx, os.Stdin, os.Stdout, func(ctx context.Context, name string, args json.RawMessage) (string, error) {
			return callTool(ctx, dir, self, name, args)
		})
	}()
	// A read of stdin cannot be cancelled: on a signal the process leaves it behind.
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
		return nil
	}
}

// serveMCP answers JSON-RPC requests, one JSON object per line, until in ends. call runs a
// tool and returns its text; an error wrapping errMCPParams is the client's mistake, any
// other is the tool's failure.
func serveMCP(ctx context.Context, in io.Reader, out io.Writer, call func(context.Context, string, json.RawMessage) (string, error)) error {
	lines := bufio.NewReader(in)
	enc := json.NewEncoder(out)
	for {
		line, err := readLine(lines)
		var req mcpRequest
		resp := mcpResponse{JSONRPC: "2.0", ID: json.RawMessage("null")}
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case errors.Is(err, errMCPLine):
			resp.Error = &mcpError{Code: mcpParseError, Message: "parse error"}
		case err != nil:
			return fmt.Errorf("read MCP request: %w", err)
		case len(bytes.TrimSpace(line)) == 0:
			continue
		case json.Unmarshal(line, &req) != nil:
			resp.Error = &mcpError{Code: mcpParseError, Message: "parse error"}
		// A notification has no id, and a response of the client's no method: neither gets an answer.
		case req.ID == nil || req.Method == "":
			continue
		default:
			resp.ID = req.ID
			resp.Result, resp.Error = answerMCP(ctx, req, call)
		}
		if err := enc.Encode(resp); err != nil {
			return fmt.Errorf("write MCP response: %w", err)
		}
	}
}

// readLine returns the next line of r. A line beyond mcpMaxLine is read to its end and comes
// back as errMCPLine, so one oversized request does not end the server.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		if len(line) <= mcpMaxLine {
			line = append(line, part...)
		}
		switch {
		case errors.Is(err, bufio.ErrBufferFull):
			continue
		// The last line may come without a newline; the next read reports the end.
		case err != nil && (!errors.Is(err, io.EOF) || len(line) == 0):
			return nil, err
		case len(line) > mcpMaxLine:
			return nil, errMCPLine
		}
		return line, nil
	}
}

func answerMCP(ctx context.Context, req mcpRequest, call func(context.Context, string, json.RawMessage) (string, error)) (any, *mcpError) {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		// A client without a version still gets an answer.
		_ = json.Unmarshal(req.Params, &params)
		return map[string]any{
			"protocolVersion": cmp.Or(params.ProtocolVersion, mcpProtocol),
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "mac-pulse", "version": version},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": mcpTools}, nil
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &mcpError{Code: mcpInvalidParams, Message: "invalid params"}
		}
		text, err := call(ctx, params.Name, params.Arguments)
		switch {
		case errors.Is(err, errMCPParams):
			return nil, &mcpError{Code: mcpInvalidParams, Message: err.Error()}
		case err != nil:
			return map[string]any{"content": []map[string]any{{"type": "text", "text": err.Error()}}, "isError": true}, nil
		}
		return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}, nil
	}
	return nil, &mcpError{Code: mcpMethodNotFound, Message: "method not found"}
}

// callTool runs one tool against a fresh sample, the way -json does.
// Every summary and top_apps call samples for two seconds; keep a sampler running
// between calls if a client ever polls.
func callTool(ctx context.Context, dir string, self ui.Self, name string, raw json.RawMessage) (string, error) {
	var args struct {
		Metric string `json:"metric"`
		N      *int   `json:"n"`
		Range  string `json:"range"`
	}
	if len(raw) > 0 {
		// Go's own words for a type mismatch name this struct; the client gets the fixed text.
		if err := json.Unmarshal(raw, &args); err != nil {
			return "", errMCPParams
		}
	}
	var result any
	switch name {
	case "summary":
		state, err := sampleState(ctx, dir, self, collector.Detail{})
		return ui.Summary(state), err
	case "top_apps":
		metric, n := cmp.Or(args.Metric, "cpu"), mcpTopApps
		if args.N != nil {
			n = *args.N
		}
		if (metric != "cpu" && metric != "memory") || n < 1 || n > mcpTopAppsMax {
			return "", fmt.Errorf("%w: top_apps takes metric cpu or memory and n from 1 to %d", errMCPParams, mcpTopAppsMax)
		}
		state, err := sampleState(ctx, dir, self, collector.Detail{})
		if err != nil {
			return "", err
		}
		result = topApps(state.Apps.Items, metric, n)
	case "history":
		history, err := store.OpenReadOnly(dir)
		if err != nil {
			return "", fmt.Errorf("open history: %w", err)
		}
		// The store checks both values against its own lists.
		h, err := history.History(args.Metric, args.Range)
		if err != nil {
			return "", fmt.Errorf("%w: %w", errMCPParams, err)
		}
		result = ui.BuildHistory(h, nil)
	default:
		return "", fmt.Errorf("%w: unknown tool %q", errMCPParams, name)
	}
	text, err := json.Marshal(result)
	if err != nil {
		return "", fmt.Errorf("encode %s: %w", name, err)
	}
	return string(text), nil
}

// topApps is the top_apps tool's answer: the first n of items by metric.
func topApps(items []ui.App, metric string, n int) any {
	if metric == "memory" {
		slices.SortStableFunc(items, func(x, y ui.App) int { return cmp.Compare(y.Memory, x.Memory) })
	} else {
		slices.SortStableFunc(items, func(x, y ui.App) int { return cmp.Compare(y.CPU, x.CPU) })
	}
	type app struct {
		Name      string  `json:"name"`
		CPU       float64 `json:"cpu"`
		Memory    uint64  `json:"memory"`
		Processes int     `json:"processes"`
	}
	top := make([]app, 0, n)
	for _, a := range items[:min(n, len(items))] {
		top = append(top, app{Name: a.Name, CPU: a.CPU, Memory: a.Memory, Processes: a.PIDCount})
	}
	return top
}
