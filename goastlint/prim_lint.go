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

package goastlint

import (
	"go/token"
	"sort"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/values"
	"github.com/aalpar/wile/werr"
)

var (
	errLintBuild       = werr.NewStaticError("analyze build error")
	errLintUnknownName = werr.NewStaticError("unknown analyzer name")
	errLintRun         = werr.NewStaticError("analyzer run error")
)

// parseAnalyzerNames collects and validates the variadic analyzer-name arguments.
// Returns an error on an improper list, a non-string element, or an unknown name.
func parseAnalyzerNames(rest values.Value) ([]*analysis.Analyzer, error) {
	tuple, ok := rest.(values.Tuple)
	if !ok {
		return nil, nil
	}
	var analyzers []*analysis.Analyzer
	for !values.IsEmptyList(tuple) {
		pair, pok := tuple.(*values.Pair)
		if !pok {
			return nil, werr.WrapForeignErrorf(werr.ErrNotAList,
				"go-analyze: malformed analyzer list")
		}
		nameVal, sok := pair.Car().(*values.String)
		if !sok {
			return nil, werr.WrapForeignErrorf(werr.ErrNotAString,
				"go-analyze: analyzer names must be strings")
		}
		a, found := analyzerRegistry[nameVal.Value]
		if !found {
			return nil, werr.WrapForeignErrorf(errLintUnknownName,
				"go-analyze: unknown analyzer %q; use go-analyze-list for available names",
				nameVal.Value)
		}
		analyzers = append(analyzers, a)
		cdr, cok := pair.Cdr().(values.Tuple)
		if !cok {
			return nil, werr.WrapForeignErrorf(werr.ErrNotAList,
				"go-analyze: malformed analyzer list")
		}
		tuple = cdr
	}
	return analyzers, nil
}

// PrimGoAnalyze implements (go-analyze target analyzer-name ...).
// target is a package pattern string or a GoSession from go-load.
// If given a GoSession loaded without 'lint mode, falls back to fresh loading.
func PrimGoAnalyze(mc machine.CallContext) error {
	analyzers, err := parseAnalyzerNames(mc.Arg(1))
	if err != nil {
		return err
	}

	if len(analyzers) == 0 {
		mc.SetValue(values.EmptyList)
		return nil
	}

	return goast.DispatchSessionOrPattern(mc.Arg(0), "go-analyze",
		func(s *goast.GoSession) error {
			if s.IsLintMode() {
				return analyzeFromSession(mc, s, analyzers)
			}
			// Non-lint session: fall back to fresh load with LoadAllSyntax.
			return analyzeFromPattern(mc, s.Patterns(), analyzers)
		},
		func(p *values.String) error {
			return analyzeFromPattern(mc, []string{p.Value}, analyzers)
		})
}

func analyzeFromSession(mc machine.CallContext, session *goast.GoSession, analyzers []*analysis.Analyzer) error {
	graph, analyzeErr := checker.Analyze(analyzers, session.Packages(), nil)
	if analyzeErr != nil {
		return werr.WrapForeignErrorf(errLintRun,
			"go-analyze: %s", analyzeErr)
	}
	return collectDiagnostics(mc, graph, session.FileSet())
}

func analyzeFromPattern(mc machine.CallContext, patterns []string, analyzers []*analysis.Analyzer) error {
	fset := token.NewFileSet()

	pkgs, err := goast.LoadPackagesChecked(mc,
		packages.LoadAllSyntax,
		fset, errLintBuild, "go-analyze",
		patterns...)
	if err != nil {
		return err
	}

	graph, analyzeErr := checker.Analyze(analyzers, pkgs, nil)
	if analyzeErr != nil {
		return werr.WrapForeignErrorf(errLintRun,
			"go-analyze: %s", analyzeErr)
	}
	return collectDiagnostics(mc, graph, fset)
}

func collectDiagnostics(mc machine.CallContext, graph *checker.Graph, fset *token.FileSet) error {
	var result []values.Value
	for _, act := range graph.Roots {
		if act.Err != nil {
			return werr.WrapForeignErrorf(errLintRun,
				"go-analyze: analyzer %q on %s: %s",
				act.Analyzer.Name, act.Package.PkgPath, act.Err)
		}
		for _, d := range act.Diagnostics {
			pos := fset.Position(d.Pos)
			fields := []values.Value{
				goast.Field("analyzer", goast.Str(act.Analyzer.Name)),
				goast.Field("pos", goast.Str(pos.String())),
				goast.Field("message", goast.Str(d.Message)),
				goast.Field("category", goast.Str(d.Category)),
			}
			result = append(result, goast.Node("diagnostic", fields...))
		}
	}
	mc.SetValue(goast.ValueList(result))
	return nil
}

// PrimGoAnalyzeList returns a sorted list of available analyzer name strings.
func PrimGoAnalyzeList(mc machine.CallContext) error {
	names := make([]string, 0, len(analyzerRegistry))
	for name := range analyzerRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	result := make([]values.Value, len(names))
	for i, name := range names {
		result[i] = goast.Str(name)
	}
	mc.SetValue(goast.ValueList(result))
	return nil
}
