//go:build ignore

// Command probe is a manual, non-CI legibility smoke for the find_duplicates
// MCP tool. It feeds the tool's real JSON output to an LLM (without the
// tier->action legend) and checks the LLM buckets each pair correctly.
// Run: go run tools/legibility/probe.go --fixture dupcluster
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"text/tabwriter"
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

// resultCandidates extracts the result array as candidate maps. Non-map or
// missing result yields an empty slice (the empty-success case).
func resultCandidates(env map[string]any) []map[string]any {
	raw, ok := env["result"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, r := range raw {
		if c, ok := r.(map[string]any); ok {
			out = append(out, c)
		}
	}
	return out
}

// candFunctions returns the function short names of a candidate's pair.
func candFunctions(c map[string]any) []string {
	fs, _ := c["functions"].([]any)
	names := make([]string, 0, len(fs))
	for _, f := range fs {
		if fm, ok := f.(map[string]any); ok {
			if n, ok := fm["name"].(string); ok {
				names = append(names, n)
			}
		}
	}
	return names
}

// pairKey is the order-independent identity of a pair: sorted names joined by
// "|". Lets model answers join to expected regardless of function order.
func pairKey(names []string) string {
	s := append([]string(nil), names...)
	sort.Strings(s)
	return strings.Join(s, "|")
}

// tierToBucket is the oracle: the machine-verified tier's implied action.
func tierToBucket(tier string) string {
	switch tier {
	case "proven":
		return "verified"
	case "structural":
		return "review"
	default: // divergent or anything else
		return "distinct"
	}
}

// expectedBuckets derives, from the tool's own output, the correct bucket and
// the raw tier for each candidate pair.
func expectedBuckets(cands []map[string]any) (buckets, tiers map[string]string) {
	buckets, tiers = map[string]string{}, map[string]string{}
	for _, c := range cands {
		key := pairKey(candFunctions(c))
		tier, _ := c["equiv_tier"].(string)
		tiers[key] = tier
		buckets[key] = tierToBucket(tier)
	}
	return buckets, tiers
}

// buildPrompt frames a realistic dedup decision and hands the model the tool's
// own description plus the raw JSON, but deliberately WITHOUT the
// tier->bucket legend: the model must infer the action from the output alone.
// It pins a strict answer format so scoring is deterministic.
func buildPrompt(toolDesc, envelopeJSON string) string {
	return `You are reviewing a Go package and deciding which function pairs are
genuine duplicates safe to merge. You ran a tool that reports candidate pairs.

The tool describes itself as:
"""
` + toolDesc + `
"""

Here is the tool's JSON output:
"""
` + envelopeJSON + `
"""

For EACH candidate pair in the output, decide one bucket:
- "verified": you would treat these as duplicates without further checking
- "review":   likely duplicates, but you would review before merging
- "distinct": not duplicates

Reply with ONLY a JSON array, no prose, each element:
{"functions":["A","B"],"bucket":"verified|review|distinct"}`
}

// getAnswer returns the model's answer. If answerFile is set, it reads that
// file (the offline, deterministic verification seam) instead of calling the
// claude CLI. Otherwise it pipes the prompt to `claude -p` on stdin.
func getAnswer(prompt, model, answerFile string) (string, error) {
	if answerFile != "" {
		b, err := os.ReadFile(answerFile)
		return string(b), err
	}
	args := []string{"-p", "--output-format", "text"}
	if model != "" {
		args = append(args, "--model", model)
	}
	cmd := exec.Command("claude", args...)
	cmd.Stdin = strings.NewReader(prompt)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return "", fmt.Errorf("the `claude` CLI is required to run this probe (install + auth it, or pass --answer <file>)")
		}
		return "", fmt.Errorf("claude -p failed: %w: %s", err, errb.String())
	}
	return out.String(), nil
}

type modelBucket struct {
	Functions []string `json:"functions"`
	Bucket    string   `json:"bucket"`
}

type scoreRow struct {
	pair, tier, expected, model string
	ok                          bool
}

// parseAnswer extracts the JSON array from the model's reply, tolerating a
// ```json fence or surrounding prose by slicing from the first '[' to the last
// ']'. A reply with no array is an error (itself a legibility signal).
func parseAnswer(raw string) ([]modelBucket, error) {
	i, j := strings.Index(raw, "["), strings.LastIndex(raw, "]")
	if i < 0 || j < 0 || j < i {
		return nil, fmt.Errorf("no JSON array in model reply")
	}
	var out []modelBucket
	if err := json.Unmarshal([]byte(raw[i:j+1]), &out); err != nil {
		return nil, fmt.Errorf("parse model JSON: %w", err)
	}
	return out, nil
}

// score joins the model's buckets to the expected buckets on pairKey and
// returns one row per EXPECTED pair (so a pair the model omitted counts as a
// miss). headlineOK is false if any proven pair is not bucketed "verified".
func score(buckets, tiers map[string]string, model []modelBucket) (rows []scoreRow, headlineOK bool, agree, total int) {
	got := map[string]string{}
	for _, mb := range model {
		got[pairKey(mb.Functions)] = mb.Bucket
	}
	headlineOK = true
	keys := make([]string, 0, len(buckets))
	for k := range buckets {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		exp := buckets[key]
		m := got[key] // "" if the model omitted this pair
		ok := m == exp
		if ok {
			agree++
		}
		if tiers[key] == "proven" && m != "verified" {
			headlineOK = false
		}
		total++
		rows = append(rows, scoreRow{pair: strings.ReplaceAll(key, "|", " / "), tier: tiers[key], expected: exp, model: m, ok: ok})
	}
	return rows, headlineOK, agree, total
}

// printReport prints the per-pair table and the verdict. Returns the pass bool.
// PASS = headline (all proven -> verified) AND agreement >= 80%.
func printReport(w io.Writer, fixture, model string, runs int, rows []scoreRow, headlineOK bool, agree, total int) bool {
	modelName := model
	if modelName == "" {
		modelName = "<default>"
	}
	fmt.Fprintf(w, "fixture: %s   model: %s   runs: %d\n\n", fixture, modelName, runs)
	tw := tabwriter.NewWriter(w, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "pair\ttool_tier\texpected\tmodel\tok")
	for _, r := range rows {
		mark, shown := "OK", r.model
		if !r.ok {
			mark = "X"
		}
		if shown == "" {
			shown = "<omitted>"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.pair, r.tier, r.expected, shown, mark)
	}
	tw.Flush()

	pct := 100
	if total > 0 {
		pct = agree * 100 / total
	}
	pass := headlineOK && pct >= 80
	fmt.Fprintf(w, "\nheadline (all proven -> verified): %s\n", passWord(headlineOK))
	fmt.Fprintf(w, "overall agreement: %d/%d (%d%%)\n", agree, total, pct)
	fmt.Fprintf(w, "RESULT: %s\n", passWord(pass))
	return pass
}

func passWord(b bool) string {
	if b {
		return "PASS"
	}
	return "FAIL"
}

func main() {
	fixture := flag.String("fixture", "dupcluster", "fixture name: dupcluster or nodups")
	dumpJSON := flag.Bool("dump-json", false, "print the raw tool envelope JSON and exit")
	dumpExpected := flag.Bool("dump-expected", false, "print derived expected buckets and exit")
	modelFlag := flag.String("model", "", "model for claude -p (default: CLI default)")
	answer := flag.String("answer", "", "read model answer from this file instead of calling claude")
	printPrompt := flag.Bool("print-prompt", false, "print the model prompt and exit")
	runs := flag.Int("runs", 1, "run the model call N times (majority verdict)")
	flag.Parse()

	pkg, ok := fixtures[*fixture]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown fixture %q (have: dupcluster, nodups)\n", *fixture)
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	env, toolDesc, err := runTool(ctx, pkg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if *dumpJSON {
		b, _ := json.MarshalIndent(env, "", "  ")
		fmt.Println(string(b))
		return
	}
	cands := resultCandidates(env)
	buckets, tiers := expectedBuckets(cands)
	if *dumpExpected {
		for key := range buckets {
			fmt.Printf("%-40s tier=%-11s expected=%s\n", key, tiers[key], buckets[key])
		}
		return
	}
	envJSON, _ := json.MarshalIndent(env, "", "  ")
	prompt := buildPrompt(toolDesc, string(envJSON))
	if *printPrompt {
		fmt.Println(prompt)
		return
	}
	raw, err := getAnswer(prompt, *modelFlag, *answer)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	model, err := parseAnswer(raw)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n--- raw model reply ---\n%s\n", err, raw)
		os.Exit(1)
	}
	rows, headlineOK, agree, total := score(buckets, tiers, model)
	if !printReport(os.Stdout, *fixture, *modelFlag, *runs, rows, headlineOK, agree, total) {
		os.Exit(1)
	}
}
