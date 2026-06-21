package internal

import (
	"errors"
	"testing"
)

func TestPrintErrln(t *testing.T) {
	PrintErrln("test", nil)
	PrintErrln("test", errors.New("an error"))
}
