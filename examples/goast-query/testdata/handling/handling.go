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
