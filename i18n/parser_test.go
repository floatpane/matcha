package i18n

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestParseJSONWrapsSyntaxError(t *testing.T) {
	_, err := ParseJSON([]byte(`{"language":"en","messages":`))
	if err == nil {
		t.Fatal("ParseJSON() error = nil, want parse error")
	}

	if !errors.Is(err, ErrParseFailed) {
		t.Fatalf("ParseJSON() error = %v, want ErrParseFailed in chain", err)
	}

	var syntaxErr *json.SyntaxError
	if !errors.As(err, &syntaxErr) {
		t.Fatalf("ParseJSON() error = %v, want json.SyntaxError in chain", err)
	}
}
