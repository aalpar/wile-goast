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

package fieldguard

import "log"

type Request struct {
	Valid bool
	Data  string
}

func HandleSafeA(r Request) string {
	if !r.Valid {
		return "invalid"
	}
	return r.Data
}

func HandleSafeB(r Request) string {
	if !r.Valid {
		log.Println("invalid request")
		return ""
	}
	return r.Data
}

func HandleSafeC(r Request) string {
	if !r.Valid {
		return "bad"
	}
	return r.Data
}

func HandleSafeD(r Request) string {
	if !r.Valid {
		return "err"
	}
	return r.Data
}

// HandleUnsafe uses r without checking Valid — intentional deviation.
func HandleUnsafe(r Request) string {
	log.Println("data:", r.Data)
	return r.Data
}
