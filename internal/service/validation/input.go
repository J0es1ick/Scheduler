package validation

import (
	"errors"
	"fmt"
)

var ErrPersonalInput = errors.New("invalid personal schedule input")

func PersonalInput(message string) error { return fmt.Errorf("%w: %s", ErrPersonalInput, message) }
