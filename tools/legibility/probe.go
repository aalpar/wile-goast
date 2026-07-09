//go:build ignore

// Command probe is a manual, non-CI legibility smoke for the find_duplicates
// MCP tool. It feeds the tool's real JSON output to an LLM (without the
// tier->action legend) and checks the LLM buckets each pair correctly.
// Run: go run tools/legibility/probe.go --fixture dupcluster
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

var fixtures = map[string]string{
	"dupcluster": "github.com/aalpar/wile-goast/examples/goast-query/testdata/dupcluster",
	"nodups":     "github.com/aalpar/wile-goast/cmd/wile-goast/testdata/nodups",
}

// runTool drives the real wile-goast binary over MCP stdio and returns the
// find_duplicates envelope for pkg plus the tool's advertised description.
// It uses the same mcp-go client the integration tests use, so the JSON is
// byte-for-byte what an agent receives (the marshal.go path, incl.
// BigFloat->float64). The binary is launched via `go run ./cmd/wile-goast
// --mcp` so no prebuild step is needed; set WILE_GOAST_BIN to a prebuilt
// binary path to skip the compile.
func runTool(ctx context.Context, pkg string) (map[string]any, string, error) {
	cmd, args := "go", []string{"run", "./cmd/wile-goast", "--mcp"}
	if bin := os.Getenv("WILE_GOAST_BIN"); bin != "" {
		cmd, args = bin, []string{"--mcp"}
	}
	c, err := client.NewStdioMCPClient(cmd, nil, args...)
	if err != nil {
		return nil, "", fmt.Errorf("start mcp server: %w", err)
	}
	defer c.Close()
	if err := c.Start(ctx); err != nil {
		return nil, "", fmt.Errorf("start client: %w", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "legibility-probe", Version: "0"}
	if _, err := c.Initialize(ctx, initReq); err != nil {
		return nil, "", fmt.Errorf("initialize: %w", err)
	}

	desc := ""
	tools, err := c.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, "", fmt.Errorf("list tools: %w", err)
	}
	for _, t := range tools.Tools {
		if t.Name == "find_duplicates" {
			desc = t.Description
		}
	}
	if desc == "" {
		return nil, "", fmt.Errorf("find_duplicates not advertised by server")
	}

	callReq := mcp.CallToolRequest{}
	callReq.Params.Name = "find_duplicates"
	callReq.Params.Arguments = map[string]any{"target": pkg}
	res, err := c.CallTool(ctx, callReq)
	if err != nil {
		return nil, "", fmt.Errorf("call find_duplicates: %w", err)
	}
	if res.IsError {
		return nil, "", fmt.Errorf("tool returned error for %q", pkg)
	}
	if m, ok := res.StructuredContent.(map[string]any); ok {
		return m, desc, nil
	}
	if len(res.Content) > 0 {
		if tc, ok := mcp.AsTextContent(res.Content[0]); ok {
			var env map[string]any
			if err := json.Unmarshal([]byte(tc.Text), &env); err != nil {
				return nil, "", fmt.Errorf("parse tool JSON: %w", err)
			}
			return env, desc, nil
		}
	}
	return nil, "", fmt.Errorf("tool returned neither structured nor text JSON")
}

func main() {
	fixture := flag.String("fixture", "dupcluster", "fixture name: dupcluster or nodups")
	dumpJSON := flag.Bool("dump-json", false, "print the raw tool envelope JSON and exit")
	flag.Parse()

	pkg, ok := fixtures[*fixture]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown fixture %q (have: dupcluster, nodups)\n", *fixture)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, _, err := runTool(ctx, pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if *dumpJSON {
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Printf("fixture %s: got envelope with keys %v\n", *fixture, keysOf(env))
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
