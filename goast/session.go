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

package goast

import (
	"go/token"
	"sync"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"

	"github.com/aalpar/wile/values"
)

// GoSession holds loaded Go packages and lazily-built analysis state.
// All package-loading primitives accept a GoSession to reuse loaded state
// instead of calling packages.Load independently.
type GoSession struct {
	patterns []string
	pkgs     []*packages.Package
	fset     *token.FileSet
	lintMode bool

	ssaOnce     sync.Once
	prog        *ssa.Program
	ssaPkgs     []*ssa.Package
	allPkgsOnce sync.Once

	cacheMu sync.Mutex
	cache   map[string]any
}

const sessionTag = "go-session"

// NewGoSession creates a GoSession from already-loaded packages.
func NewGoSession(patterns []string, pkgs []*packages.Package, fset *token.FileSet, lintMode bool) *GoSession {
	return &GoSession{
		patterns: patterns,
		pkgs:     pkgs,
		fset:     fset,
		lintMode: lintMode,
		cache:    make(map[string]any),
	}
}

// WrapSession wraps a GoSession as an OpaqueValue for Scheme.
func WrapSession(s *GoSession) *values.OpaqueValue {
	return values.NewOpaqueValue(sessionTag, s)
}

// UnwrapSession extracts a GoSession from a values.Value.
// Returns nil, false if v is not a go-session OpaqueValue.
func UnwrapSession(v values.Value) (*GoSession, bool) {
	o, ok := v.(*values.OpaqueValue)
	if !ok || o.OpaqueTag() != sessionTag {
		return nil, false
	}
	s, ok := o.Unwrap().(*GoSession)
	return s, ok
}

// Packages returns the loaded packages.
func (s *GoSession) Packages() []*packages.Package { return s.pkgs }

// FileSet returns the shared token.FileSet.
func (s *GoSession) FileSet() *token.FileSet { return s.fset }

// IsLintMode returns true if loaded with LoadAllSyntax.
func (s *GoSession) IsLintMode() bool { return s.lintMode }

// Patterns returns the root patterns used to load this session.
func (s *GoSession) Patterns() []string { return s.patterns }

// SSA lazily builds SSA for the requested packages.
func (s *GoSession) SSA() (*ssa.Program, []*ssa.Package) {
	s.ssaOnce.Do(func() {
		s.prog, s.ssaPkgs = ssautil.Packages(s.pkgs,
			ssa.SanityCheckFunctions|ssa.InstantiateGenerics)
		for _, pkg := range s.ssaPkgs {
			if pkg != nil {
				pkg.Build()
			}
		}
	})
	return s.prog, s.ssaPkgs
}

// SSAAllPackages lazily builds SSA for all transitively loaded packages.
// Required by callgraph algorithms that need cross-package edges.
func (s *GoSession) SSAAllPackages() *ssa.Program {
	prog, _ := s.SSA()
	s.allPkgsOnce.Do(func() {
		for _, pkg := range prog.AllPackages() {
			pkg.Build()
		}
	})
	return prog
}

// CachedValue retrieves a cached value by key. Sub-extensions use this
// to cache derived data (e.g., callgraphs) without GoSession importing
// their packages.
func (s *GoSession) CachedValue(key string) (any, bool) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	v, ok := s.cache[key]
	return v, ok
}

// SetCachedValue stores a value in the session cache.
func (s *GoSession) SetCachedValue(key string, v any) {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache[key] = v
}
