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

package sameblock

func Foo() int { return 1 }
func Bar() int { return 2 }

func FooFirst() int {
	a := Foo()
	b := Bar()
	return a + b
}

func BarFirst() int {
	b := Bar()
	a := Foo()
	return a + b
}
