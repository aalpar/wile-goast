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

// Package reachability is a fixture for the reaches-call checker: a target
// function reached directly, one hop away, and two hops away, plus one caller
// that never reaches it (the intended deviation).
package reachability

// sink is a package-level effect so the fixture calls are not optimized away.
var sink int

// Target is the function the belief requires each site to reach.
func Target() {
	sink++
}

// helper is an off-path leaf — calling it does not reach Target.
func helper() {
	sink--
}

// Direct reaches Target with a direct call.
func Direct() {
	Target()
}

// OneHop reaches Target transitively through Direct.
func OneHop() {
	Direct()
}

// TwoHop reaches Target transitively through OneHop -> Direct.
func TwoHop() {
	OneHop()
}

// Bad calls only helper — it never reaches Target. Intentional deviation.
func Bad() {
	helper()
}
