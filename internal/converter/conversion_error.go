package converter

import "errors"

var ErrNilConvertedResponse = errors.New("converted response is nil")

// ResponseConversionError marks failures while converting an upstream response
// back into the client protocol. Callers should fail closed instead of passing
// the upstream protocol through to a client that cannot understand it.
type ResponseConversionError struct {
	Err error
}

func (e *ResponseConversionError) Error() string {
	if e == nil || e.Err == nil {
		return "response format conversion failed"
	}
	return "response format conversion failed: " + e.Err.Error()
}

func (e *ResponseConversionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func NewResponseConversionError(err error) error {
	if err == nil {
		return nil
	}
	return &ResponseConversionError{Err: err}
}

func IsResponseConversionError(err error) bool {
	var target *ResponseConversionError
	return errors.As(err, &target)
}
