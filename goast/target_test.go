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
	"testing"

	"github.com/aalpar/wile/values"
)

func TestCurrentGoTargetDefault(t *testing.T) {
	ResetTargetState()
	t.Setenv(targetEnvVar, "")
	p := GetCurrentGoTargetParam()
	if p == nil {
		t.Fatal("GetCurrentGoTargetParam returned nil")
	}
	s, ok := p.Value().(*values.String)
	if !ok {
		t.Fatalf("parameter value is %T, want *values.String", p.Value())
	}
	if s.Value != targetDefaultPattern {
		t.Errorf("default value = %q, want %q", s.Value, targetDefaultPattern)
	}
}

func TestCurrentGoTargetEnvOverride(t *testing.T) {
	ResetTargetState()
	t.Setenv(targetEnvVar, "github.com/example/foo/...")
	p := GetCurrentGoTargetParam()
	s, ok := p.Value().(*values.String)
	if !ok {
		t.Fatalf("parameter value is %T, want *values.String", p.Value())
	}
	if s.Value != "github.com/example/foo/..." {
		t.Errorf("env-override value = %q, want %q",
			s.Value, "github.com/example/foo/...")
	}
}

func TestCurrentGoTargetIdempotentInit(t *testing.T) {
	ResetTargetState()
	first := GetCurrentGoTargetParam()
	second := GetCurrentGoTargetParam()
	if first != second {
		t.Errorf("repeated InitTargetState returned different parameters: %p vs %p",
			first, second)
	}
}
