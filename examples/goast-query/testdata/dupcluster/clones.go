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

package dupcluster

import "sort"

// SumSlice and TotalSlice are structural clones (a unifiable pair) that share
// the rare ref "sort", so they cluster together under FCA-on-references.

func SumSlice(xs []int) int {
	sort.Ints(xs)
	total := 0
	for _, x := range xs {
		total += x
	}
	return total
}

func TotalSlice(ys []int) int {
	sort.Ints(ys)
	sum := 0
	for _, y := range ys {
		sum += y
	}
	return sum
}
