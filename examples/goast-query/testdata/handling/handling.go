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

package handling

import "fmt"

func DoWork(input string) (string, error) {
	if input == "" {
		return "", fmt.Errorf("empty input")
	}
	return input + " done", nil
}

func CallerA(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerA: %w", err)
	}
	return result, nil
}

func CallerB(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerB: %w", err)
	}
	return result, nil
}

func CallerC(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerC: %w", err)
	}
	return result, nil
}

func CallerD(input string) (string, error) {
	result, err := DoWork(input)
	if err != nil {
		return "", fmt.Errorf("callerD: %w", err)
	}
	return result, nil
}

// CallerBad ignores the error from DoWork — intentional deviation.
func CallerBad(input string) string {
	result, _ := DoWork(input)
	return result
}
