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
	"os"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/aalpar/wile/pkg/machine"
	"github.com/aalpar/wile/pkg/security"
	"github.com/aalpar/wile/pkg/werr"
)

// LoadPackagesChecked performs a security-checked packages.Load with
// standard error aggregation. It unifies the repeated pattern of
// security check + packages.Load + pkg.Errors collection found across
// all five sub-extensions.
//
// dir, when non-empty, sets packages.Config.Dir so patterns resolve against
// that directory instead of the host process's working directory. It also
// gates an additional ResourceFile/ActionRead security check on dir and a
// pre-check that dir exists and is a directory, so a bad path fails with a
// clear message instead of packages.Load's opaque one. Callers that are
// deliberately CWD-bound pass "", which leaves behavior unchanged.
func LoadPackagesChecked(
	mc machine.CallContext,
	mode packages.LoadMode,
	fset *token.FileSet,
	sentinel error,
	command string,
	dir string,
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

	if dir != "" {
		err = security.CheckWithAuthorizer(mc.Authorizer(), security.AccessRequest{
			Resource: security.ResourceFile,
			Action:   security.ActionRead,
			Target:   dir,
		})
		if err != nil {
			return nil, err
		}

		info, statErr := os.Stat(dir)
		if statErr != nil {
			return nil, werr.WrapForeignErrorf(sentinel,
				"%s: dir %q: %s", command, dir, statErr)
		}
		if !info.IsDir() {
			return nil, werr.WrapForeignErrorf(sentinel,
				"%s: dir %q is not a directory", command, dir)
		}
	}

	cfg := &packages.Config{
		Mode:    mode,
		Context: mc.Context(),
	}
	if fset != nil {
		cfg.Fset = fset
	}
	if dir != "" {
		cfg.Dir = dir
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
