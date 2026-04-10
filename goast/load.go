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
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/machine"
	"github.com/aalpar/wile/security"
	"github.com/aalpar/wile/werr"
)

// LoadPackagesChecked performs a security-checked packages.Load with
// standard error aggregation. It unifies the repeated pattern of
// security check + packages.Load + pkg.Errors collection found across
// all five sub-extensions.
func LoadPackagesChecked(
	mc machine.CallContext,
	mode packages.LoadMode,
	fset *token.FileSet,
	sentinel error,
	command string,
	patterns ...string,
) ([]*packages.Package, error) {
	err := security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
		Resource: security.ResourceProcess,
		Action:   security.ActionLoad,
		Target:   "go",
	})
	if err != nil {
		return nil, err
	}

	cfg := &packages.Config{
		Mode:    mode,
		Context: mc.Context(),
	}
	if fset != nil {
		cfg.Fset = fset
	}

	pkgs, loadErr := packages.Load(cfg, patterns...)
	if loadErr != nil {
		return nil, werr.WrapForeignErrorf(sentinel, "%s: %s", command, loadErr)
	}

	var errs []string
	for _, pkg := range pkgs {
		for _, e := range pkg.Errors {
			errs = append(errs, e.Error())
		}
	}
	if len(errs) > 0 {
		return nil, werr.WrapForeignErrorf(sentinel, "%s: %s", command, strings.Join(errs, "; "))
	}

	return pkgs, nil
}
