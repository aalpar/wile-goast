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
