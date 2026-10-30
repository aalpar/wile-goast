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

package b

import (
	"strconv"
	"strings"
	"unicode"
)

// Parse shares only its name with a.Parse.
func Parse(s string) int {
	_ = unicode.IsDigit
	if strings.HasPrefix(s, "#") {
		return -1
	}
	v, err := strconv.ParseBool(s)
	if err != nil || !v {
		return 0
	}
	return 1
}
