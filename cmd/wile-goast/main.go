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

// Command wile-goast demonstrates using Go AST analysis extensions with Wile.
// It creates an engine with all goast extensions loaded, then evaluates
// each -e expression or runs interactively if none are provided.
package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"os"
	"strings"

	"github.com/aalpar/wile"
	"github.com/aalpar/wile-goast/goast"
	"github.com/aalpar/wile-goast/goastcfg"
	"github.com/aalpar/wile-goast/goastcg"
	"github.com/aalpar/wile-goast/goastlint"
	"github.com/aalpar/wile-goast/goastssa"
)

func main() {
	ctx := context.Background()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--list-scripts":
			listScripts()
			return
		case "--run":
			if len(os.Args) < 3 {
				fmt.Fprintln(os.Stderr, "Usage: wile-goast --run <script-name>")
				os.Exit(1)
			}
			runScript(ctx, os.Args[2])
			return
		}
	}

	engine := buildEngine(ctx)
	defer func() {
		_ = engine.Close()
	}()

	// If arguments are provided, evaluate them as Scheme expressions.
	if len(os.Args) > 1 {
		code := strings.Join(os.Args[1:], " ")
		val, err := engine.Eval(ctx, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		if val != nil {
			fmt.Println(val)
		}
		return
	}

	// No arguments: print available extensions.
	fmt.Println("wile-goast: Wile Scheme with Go AST analysis extensions")
	fmt.Println()
	fmt.Println("Available extensions:")
	fmt.Println("  goast      — parse, format, type-check Go source as s-expressions")
	fmt.Println("  goast-ssa  — SSA (Static Single Assignment) analysis")
	fmt.Println("  goast-cfg  — control flow graph, dominators, path enumeration")
	fmt.Println("  goast-cg   — call graph construction (static, CHA, RTA)")
	fmt.Println("  goast-lint — go/analysis passes")
	fmt.Println()
	fmt.Println("Usage: wile-goast '<scheme-expression>'")
	fmt.Println("       wile-goast --list-scripts")
	fmt.Println("       wile-goast --run <script-name>")
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

func listScripts() {
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

func runScript(ctx context.Context, name string) {
	scriptPath := "scripts/" + name + ".scm"
	data, err := fs.ReadFile(embeddedScripts, scriptPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Script %q not found. Use --list-scripts to see available scripts.\n", name)
		os.Exit(1)
	}

	engine := buildEngine(ctx)
	defer func() { _ = engine.Close() }()

	val, evalErr := engine.Eval(ctx, string(data))
	if evalErr != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", evalErr)
		os.Exit(1)
	}
	if val != nil {
		fmt.Println(val)
	}
}
