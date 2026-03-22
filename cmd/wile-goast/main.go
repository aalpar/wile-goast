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

// Command wile-goast provides Go static analysis extensions for the Wile
// Scheme interpreter. It supports the same execution modes as wile:
//
//	wile-goast -e '(+ 1 2)'            evaluate expression
//	wile-goast -f script.scm            run file
//	wile-goast -f lib.scm -e '(run)'    load file, then evaluate
//	echo '(+ 1 2)' | wile-goast         read from stdin
//	wile-goast script.scm               positional file argument
//	wile-goast --run belief-example      run embedded script
//	wile-goast --list-scripts            list embedded scripts
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/aalpar/wile"
	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/goastcfg"
	"github.com/aalpar/wile-goast/goastcg"
	"github.com/aalpar/wile-goast/goastlint"
	"github.com/aalpar/wile-goast/goastssa"
)

// stringSlice implements flag.Value for repeatable string flags (-e, -f).
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ", ") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

var (
	evalExprs   stringSlice
	files       stringSlice
	listScripts bool
	runScript   string
)

func init() {
	flag.Var(&evalExprs, "e", "Evaluate Scheme expression (repeatable)")
	flag.Var(&files, "f", "Scheme file to load (repeatable)")
	flag.BoolVar(&listScripts, "list-scripts", false, "List available embedded scripts")
	flag.StringVar(&runScript, "run", "", "Run an embedded script by name")

	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: wile-goast [OPTIONS] [FILE]")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Go static analysis extensions for Wile Scheme.")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Extensions: goast, goast-ssa, goast-cfg, goast-cg, goast-lint")
	}
}

func main() {
	flag.Parse()
	ctx := context.Background()

	// --list-scripts: no engine needed
	if listScripts {
		doListScripts()
		return
	}

	// --run: run an embedded script
	if runScript != "" {
		doRunScript(ctx, runScript)
		return
	}

	// Positional argument as file if -f not specified
	if len(files) == 0 && flag.NArg() > 0 {
		files = append(files, flag.Arg(0))
	}

	// No flags, no files, no positional args, no stdin pipe → usage
	if len(files) == 0 && len(evalExprs) == 0 && !stdinIsPipe() {
		flag.Usage()
		return
	}

	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	// Load files (-f or positional)
	for _, filename := range files {
		if err := loadFile(ctx, engine, filename); err != nil {
			fmt.Fprintf(os.Stderr, "Error loading %s: %v\n", filename, err)
			os.Exit(1)
		}
	}

	// Evaluate -e expressions
	for _, expr := range evalExprs {
		if err := evalAndPrint(ctx, engine, expr); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}

	// Read from stdin if piped and no files/evals were provided
	if len(files) == 0 && len(evalExprs) == 0 && stdinIsPipe() {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
			os.Exit(1)
		}
		if err := evalAndPrint(ctx, engine, string(data)); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	}
}

func buildEngine(ctx context.Context) *wile.Engine {
	engine, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithSourceFS(embeddedLib),
		wile.WithLibraryPaths("lib"),
		wile.WithExtension(goast.Extension),
		wile.WithExtension(goastssa.Extension),
		wile.WithExtension(goastcg.Extension),
		wile.WithExtension(goastcfg.Extension),
		wile.WithExtension(goastlint.Extension),
	)
	if err != nil {
		log.Fatal(err)
	}
	return engine
}

func loadFile(ctx context.Context, engine *wile.Engine, filename string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return err
	}
	return engine.WithLoadPath(absPath, func() error {
		val, evalErr := engine.EvalMultipleWithSource(ctx, string(data), filename)
		if evalErr != nil {
			return evalErr
		}
		if val != nil && !val.IsVoid() {
			fmt.Println(val.SchemeString())
		}
		return nil
	})
}

func evalAndPrint(ctx context.Context, engine *wile.Engine, code string) error {
	val, err := engine.EvalMultiple(ctx, code)
	if err != nil {
		return err
	}
	if val != nil && !val.IsVoid() {
		fmt.Println(val.SchemeString())
	}
	return nil
}

func doListScripts() {
	entries, err := fs.ReadDir(embeddedScripts, "scripts")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading scripts: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Available scripts:")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".scm") {
			name := strings.TrimSuffix(e.Name(), ".scm")
			fmt.Printf("  %s\n", name)
		}
	}
}

func doRunScript(ctx context.Context, name string) {
	scriptPath := "scripts/" + name + ".scm"
	data, err := fs.ReadFile(embeddedScripts, scriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Script %q not found. Use --list-scripts to see available scripts.\n", name)
		os.Exit(1)
	}

	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	val, evalErr := engine.EvalMultiple(ctx, string(data))
	if evalErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", evalErr)
		os.Exit(1)
	}
	if val != nil && !val.IsVoid() {
		fmt.Println(val.SchemeString())
	}
}

func stdinIsPipe() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}
