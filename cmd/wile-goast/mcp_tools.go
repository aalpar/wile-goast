// Copyright 2026 Aaron Alpar
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// registerPhase1Tools registers the Phase 1 pipeline tools on s. Called
// from newServer (the shared stdio+HTTP construction site), so both
// transports expose the same tools.
func (ms *mcpServer) registerPhase1Tools(s *server.MCPServer) {
	s.AddTool(
		mcp.NewTool("check_beliefs",
			mcp.WithDescription("Run committed beliefs against a Go package. "+
				"Returns an adherence/deviation report per belief. Use when you "+
				"have a .scm file or directory of belief files and want a "+
				"structural consistency report."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithString("beliefs_path", mcp.Required(),
				mcp.Description("Path to a .scm file or directory of .scm files")),
		),
		ms.handleCheckBeliefs,
	)
	s.AddTool(
		mcp.NewTool("discover_beliefs",
			mcp.WithDescription("Run a directory of discovery beliefs against a Go "+
				"package, suppress any that match a committed belief, and return the "+
				"survivors as Scheme source ready to commit. Use to mine a codebase for "+
				"new consistency patterns without re-surfacing already-committed ones."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithString("discovery_path", mcp.Required(),
				mcp.Description("Path to a discovery .scm file or directory of them")),
			mcp.WithString("committed_path",
				mcp.Description("Path to committed beliefs (optional). Omit or pass "+
					"\"\" to disable suppression and return raw discovery output.")),
		),
		ms.handleDiscoverBeliefs,
	)
	s.AddTool(
		mcp.NewTool("recommend_split",
			mcp.WithDescription("Analyze a Go package's cohesion and recommend a "+
				"two-way split via IDF-weighted FCA + min-cut. Returns the split "+
				"proposal with a confidence verdict (HIGH/MEDIUM/LOW/NONE)."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithNumber("idf_threshold",
				mcp.Description("Minimum IDF to keep a package as a signature attribute (default 0.36)")),
			mcp.WithBoolean("refine",
				mcp.Description("Refine the context by (package, object) granularity")),
			mcp.WithNumber("max_attributes",
				mcp.Description("Fail fast if attribute count exceeds this (default 30; lattice is 2^N)")),
		),
		ms.handleRecommendSplit,
	)
	s.AddTool(
		mcp.NewTool("recommend_boundaries",
			mcp.WithDescription("Recommend function boundary changes (split / merge / "+
				"extract) for a Go package via FCA over SSA struct-field access. "+
				"Returns three Pareto frontiers of candidates."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithString("mode",
				mcp.Description("Field-access mode: write-only (default), read-write, or type-only")),
		),
		ms.handleRecommendBoundaries,
	)
	s.AddTool(
		mcp.NewTool("find_duplicates",
			mcp.WithDescription("Are functions in this package semantic duplicates? "+
				"Scans a Go package for duplicate function pairs, verifying "+
				"equivalence over canonicalized SSA and AST rather than token "+
				"similarity: answers what grep/dupl and gopls cannot. Returns only "+
				"the pairs that unify (tier proven or structural), scored and "+
				"located; pass verdict=true for an opt-in duplicate/likely-duplicate "+
				"label. Pairs the unifier rejected are counted, not listed, under "+
				"provenance.tiers. An empty result is a success: no duplicates. "+
				"Refuses a target yielding over 25000 candidate pairs in about a "+
				"second rather than scoring it: narrow a '/...' tree to one package."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithNumber("threshold",
				mcp.Description("Similarity/unifiability threshold (default 0.6). Sets each "+
					"pair's equivalence tier, and so selects what is returned: lower it "+
					"to admit looser pairs, raise it to admit only near-identical ones.")),
			mcp.WithBoolean("verdict",
				mcp.Description("Attach the opt-in categorical verdict "+
					"(duplicate/likely-duplicate/distinct) per candidate (default false)")),
			mcp.WithString("scope",
				mcp.Description("Only score pairs where one function's file matches this "+
					"path glob ('*' only). Matches the whole path or any '/'-aligned "+
					"suffix: 'marshal.go', 'goast/marshal.go', '*/goast/*.go'. Pushed "+
					"down before scoring, so a scoped query is cheaper and can be "+
					"answered on a target that would otherwise exceed the pair budget.")),
			mcp.WithString("symbol",
				mcp.Description("Only score pairs where one function's name matches this "+
					"glob ('*' only). Matches the whole name or any '.'-aligned suffix, "+
					"so 'marshalValue' matches 'ssaMapper.marshalValue'. A pair survives "+
					"if either function matches.")),
			mcp.WithString("detail",
				mcp.Description("Row width: summary (default) drops the measures "+
					"sub-object; full returns it.")),
			mcp.WithNumber("limit",
				mcp.Description("Maximum rows returned (default 50). provenance.truncated "+
					"says whether rows were withheld.")),
			mcp.WithNumber("offset",
				mcp.Description("Rows skipped before limit applies (default 0).")),
		),
		ms.handleFindDuplicates,
	)
	s.AddTool(
		mcp.NewTool("find_false_boundaries",
			mcp.WithDescription("Find struct boundaries whose removal would enable "+
				"unification: FCA concepts spanning multiple struct types, annotated "+
				"with lattice relationships. Returns the cross-boundary report."),
			mcp.WithString("target", mcp.Required(),
				mcp.Description("Go package pattern (e.g., 'my/pkg/...')")),
			mcp.WithString("mode",
				mcp.Description("Field-access mode: write-only (default), read-write, or type-only")),
			mcp.WithNumber("min_extent",
				mcp.Description("Minimum concept extent (object count) to report (default 2)")),
			mcp.WithNumber("min_intent",
				mcp.Description("Minimum concept intent (attribute count) to report (default 2)")),
			mcp.WithNumber("min_types",
				mcp.Description("Minimum distinct struct types a concept must span (default 2)")),
			mcp.WithString("scope",
				mcp.Description("Only report rows whose function matches this path glob "+
					"('*' only). Matches the whole path or any '/'-aligned suffix, so "+
					"'marshal.go', 'goast/marshal.go' and '*/goast/*.go' all work.")),
			mcp.WithString("symbol",
				mcp.Description("Only report rows whose function name matches this glob "+
					"('*' only). Matches the whole name or any '.'-aligned suffix, so a "+
					"bare 'cloneTypeExpr' matches the package-qualified form. A concept "+
					"survives if any one extent member matches.")),
			mcp.WithString("detail",
				mcp.Description("Row width: summary (default) drops the lattice "+
					"relationship lists; full returns them. types_count is present "+
					"at both levels.")),
			mcp.WithNumber("limit",
				mcp.Description("Maximum rows returned (default 50). provenance.truncated "+
					"says whether rows were withheld.")),
			mcp.WithNumber("offset",
				mcp.Description("Rows skipped before limit applies (default 0).")),
		),
		ms.handleFindFalseBoundaries,
	)
	s.AddTool(
		mcp.NewTool("scheme_defs",
			mcp.WithDescription("Jump-to-definition for Scheme source. Given a "+
				"symbol, returns every definition site (define/define-syntax/"+
				"define-values, exploded define-record-type names, and library "+
				"exports) across the given roots, with file/line/col. Lexical "+
				"(pre-macro-expansion); an unresolved re-export is reported, not "+
				"silently missed."),
			mcp.WithString("symbol", mcp.Required(),
				mcp.Description("The identifier to locate (e.g., 'cfl-solve')")),
			mcp.WithString("roots", mcp.Required(),
				mcp.Description("Comma-separated directory roots to scan for "+
					".scm/.sld (e.g., 'lib,../wile/pkg/stdlib')")),
		),
		ms.handleSchemeDefs,
	)
	s.AddTool(
		mcp.NewTool("scheme_symbols",
			mcp.WithDescription("Structure overview of one Scheme file: every "+
				"top-level definition (ordered by line) and, for a .sld, its "+
				"library export list. Orientation before diving into a file."),
			mcp.WithString("file", mcp.Required(),
				mcp.Description("Path to a single .scm or .sld file")),
		),
		ms.handleSchemeSymbols,
	)
}

// boundaryMode validates a field-access mode parameter and returns the
// Scheme argument form: a quoted symbol for an allowed mode, "#f" for an
// empty (unset) mode, or ("", false) for an invalid value.
func boundaryMode(mode string) (string, bool) {
	switch mode {
	case "":
		return "#f", true
	case "write-only", "read-write", "type-only":
		return "'" + mode, true
	default:
		return "", false
	}
}

// viewOpts validates the five shared result-view parameters and returns the
// Scheme (cons 'key val) fragments for those the caller actually set, or a
// non-empty error message.
//
// An unset parameter emits nothing. Every default lives in the Scheme
// pipeline (view-default-limit, view-default-detail), so there is exactly one
// place a default is written and it is testable without an MCP round trip.
//
// RequireFloat accepts both float64 (JSON transport) and int (the in-process
// test client), so the integrality check is done here rather than delegated
// to RequireInt, which would silently truncate limit=2.5.
func viewOpts(req mcp.CallToolRequest) ([]string, string) {
	args := req.GetArguments()
	var parts []string

	scope := req.GetString("scope", "")
	if scope != "" {
		parts = append(parts, "(cons 'scope "+schemeStringLiteral(scope)+")")
	}
	symbol := req.GetString("symbol", "")
	if symbol != "" {
		parts = append(parts, "(cons 'symbol "+schemeStringLiteral(symbol)+")")
	}
	switch s := req.GetString("detail", ""); s {
	case "": // unset
	case "summary", "full":
		parts = append(parts, "(cons 'detail '"+s+")")
	default:
		return nil, "detail must be summary or full"
	}
	for _, name := range []string{"limit", "offset"} {
		_, present := args[name]
		if !present {
			continue
		}
		f, err := req.RequireFloat(name)
		if err != nil || f < 0 || f != math.Trunc(f) || f > math.MaxInt32 {
			return nil, name + " must be a non-negative integer"
		}
		parts = append(parts, fmt.Sprintf("(cons '%s %d)", name, int64(f)))
	}
	return parts, ""
}

// invokePipeline evaluates a pipeline call on the session's engine,
// marshals the resulting Wile value to JSON-compatible Go via
// marshalToJSON, and returns a tool result with both text (JSON string)
// and structured content populated. Tool-level errors become
// mcp.NewToolResultError.
//
// Mirrors handleEval's per-session-engine access (mcp.go): resolve the
// entry by session key, hold evalMu across EvalMultiple so concurrent
// pipelined requests on one client don't race the single engine.
func (ms *mcpServer) invokePipeline(ctx context.Context, code string) (*mcp.CallToolResult, error) {
	entry, err := ms.builtEntry(ctx, sessionKey(ctx))
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("engine init failed: %v", err)), nil
	}

	entry.evalMu.Lock()
	defer entry.evalMu.Unlock()

	val, err := entry.engine.EvalMultiple(ctx, code)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	data, err := marshalToJSON(val)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("marshal: %v", err)), nil
	}
	res, err := mcp.NewToolResultJSON(data)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("encode: %v", err)), nil
	}
	return res, nil
}

// schemeStringLiteral quotes s as a Scheme string literal, escaping
// backslashes and double quotes. Used to pass user-supplied paths and
// patterns safely into pipeline call source.
func schemeStringLiteral(s string) string {
	r := strings.ReplaceAll(s, `\`, `\\`)
	r = strings.ReplaceAll(r, `"`, `\"`)
	return `"` + r + `"`
}

func (ms *mcpServer) handleCheckBeliefs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	beliefsPath := req.GetString("beliefs_path", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	if beliefsPath == "" {
		return mcp.NewToolResultError("beliefs_path parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-check-beliefs ` + schemeStringLiteral(target) + ` ` + schemeStringLiteral(beliefsPath) + `)`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleDiscoverBeliefs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	discoveryPath := req.GetString("discovery_path", "")
	committedPath := req.GetString("committed_path", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	if discoveryPath == "" {
		return mcp.NewToolResultError("discovery_path parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-discover-beliefs ` +
		schemeStringLiteral(target) + ` ` +
		schemeStringLiteral(discoveryPath) + ` ` +
		schemeStringLiteral(committedPath) + `)`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleRecommendSplit(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	// Build an alist of option overrides from the supplied params; the
	// Scheme pipeline flattens it to recommend-split's plist form.
	args := req.GetArguments()
	var optsParts []string
	idf, ok := args["idf_threshold"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'idf-threshold %v)", idf))
	}
	refine, ok := args["refine"]
	if ok {
		b, _ := refine.(bool)
		if b {
			optsParts = append(optsParts, "(cons 'refine #t)")
		}
	}
	maxAttrs, ok := args["max_attributes"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'max-attributes %v)", maxAttrs))
	}
	code := `(import (wile goast pipelines))
(pipeline-recommend-split ` + schemeStringLiteral(target) +
		` (list ` + strings.Join(optsParts, " ") + `))`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleRecommendBoundaries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	modeArg, ok := boundaryMode(req.GetString("mode", ""))
	if !ok {
		return mcp.NewToolResultError("mode must be write-only, read-write, or type-only"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-recommend-boundaries ` + schemeStringLiteral(target) + ` ` + modeArg + `)`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleFindFalseBoundaries(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	mode := req.GetString("mode", "")
	modeArg, ok := boundaryMode(mode)
	if !ok {
		return mcp.NewToolResultError("mode must be write-only, read-write, or type-only"), nil
	}
	args := req.GetArguments()
	var optsParts []string
	if mode != "" {
		optsParts = append(optsParts, "(cons 'mode "+modeArg+")")
	}
	v, ok := args["min_extent"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'min-extent %v)", v))
	}
	v, ok = args["min_intent"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'min-intent %v)", v))
	}
	v, ok = args["min_types"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'min-types %v)", v))
	}
	viewParts, verr := viewOpts(req)
	if verr != "" {
		return mcp.NewToolResultError(verr), nil
	}
	optsParts = append(optsParts, viewParts...)
	code := `(import (wile goast pipelines))
(pipeline-find-false-boundaries ` + schemeStringLiteral(target) +
		` (list ` + strings.Join(optsParts, " ") + `))`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleFindDuplicates(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	target := req.GetString("target", "")
	if target == "" {
		return mcp.NewToolResultError("target parameter is required"), nil
	}
	args := req.GetArguments()
	var optsParts []string
	v, ok := args["threshold"]
	if ok {
		optsParts = append(optsParts, fmt.Sprintf("(cons 'threshold %v)", v))
	}
	v, ok = args["verdict"]
	if ok {
		b, _ := v.(bool)
		if b {
			optsParts = append(optsParts, "(cons 'verdict #t)")
		}
	}
	viewParts, verr := viewOpts(req)
	if verr != "" {
		return mcp.NewToolResultError(verr), nil
	}
	optsParts = append(optsParts, viewParts...)
	code := `(import (wile goast pipelines))
(pipeline-find-duplicates ` + schemeStringLiteral(target) +
		` (list ` + strings.Join(optsParts, " ") + `))`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleSchemeDefs(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol := req.GetString("symbol", "")
	roots := req.GetString("roots", "")
	if symbol == "" {
		return mcp.NewToolResultError("symbol parameter is required"), nil
	}
	if roots == "" {
		return mcp.NewToolResultError("roots parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-scheme-defs ` + schemeStringLiteral(symbol) + ` ` + schemeStringLiteral(roots) + `)`
	return ms.invokePipeline(ctx, code)
}

func (ms *mcpServer) handleSchemeSymbols(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	file := req.GetString("file", "")
	if file == "" {
		return mcp.NewToolResultError("file parameter is required"), nil
	}
	code := `(import (wile goast pipelines))
(pipeline-scheme-symbols ` + schemeStringLiteral(file) + `)`
	return ms.invokePipeline(ctx, code)
}
