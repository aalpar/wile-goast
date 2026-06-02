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

// Package dupcluster is a synthetic fixture for FCA-on-references duplicate
// candidate detection: EncodeA/B/C share encoding/json; LogX/LogY share log.
package dupcluster

import (
	"encoding/json"
	"log"
)

func EncodeA(v interface{}) ([]byte, error) { return json.Marshal(v) }

func EncodeB(v interface{}) ([]byte, error) {
	b, err := json.Marshal(v)
	return b, err
}

func EncodeC(v interface{}) ([]byte, error) { return json.Marshal(v) }

func LogX(s string) { log.Println(s) }

func LogY(s string) { log.Print(s) }
