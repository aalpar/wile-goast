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

package ordering

import "errors"

var ErrInvalid = errors.New("invalid")

func Validate(data string) error {
	if len(data) == 0 {
		return ErrInvalid
	}
	return nil
}

func Process(data string) string {
	return data + " processed"
}

func PipelineA(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

func PipelineB(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

func PipelineC(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

func PipelineD(data string) (string, error) {
	if err := Validate(data); err != nil {
		return "", err
	}
	return Process(data), nil
}

// PipelineReversed calls Process before Validate — intentional deviation.
func PipelineReversed(data string) (string, error) {
	result := Process(data)
	if err := Validate(data); err != nil {
		return "", err
	}
	return result, nil
}
