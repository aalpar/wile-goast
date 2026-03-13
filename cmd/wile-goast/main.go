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

	engine, err := wile.NewEngine(ctx,
		wile.WithSafeExtensions(),
		wile.WithExtension(goast.Extension),
		wile.WithExtension(goastssa.Extension),
		wile.WithExtension(goastcg.Extension),
		wile.WithExtension(goastcfg.Extension),
		wile.WithExtension(goastlint.Extension),
	)
	if err != nil {
		log.Fatal(err)
	}
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
}
