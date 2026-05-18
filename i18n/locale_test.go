package i18n

import (
	"errors"
	"testing"

	"golang.org/x/text/language"
)

func TestParseLocale(t *testing.T) {
	RegisterLanguage(&Locale{
		Tag:        language.Arabic,
		Code:       "ar",
		Name:       "Arabic",
		NativeName: "Arabic",
		Direction:  "rtl",
		PluralFunc: ArabicPlural,
	})

	tests := []struct {
		name          string
		code          string
		wantCode      string
		wantDirection string
		wantErr       error
	}{
		{
			name:          "language code",
			code:          "en",
			wantCode:      "en",
			wantDirection: "ltr",
		},
		{
			name:          "hyphenated region",
			code:          "en-US",
			wantCode:      "en",
			wantDirection: "ltr",
		},
		{
			name:          "underscored region",
			code:          "en_US",
			wantCode:      "en",
			wantDirection: "ltr",
		},
		{
			name:          "unregistered language fallback",
			code:          "eo",
			wantCode:      "eo",
			wantDirection: "ltr",
		},
		{
			name:          "registered rtl language",
			code:          "ar",
			wantCode:      "ar",
			wantDirection: "rtl",
		},
		{
			name:    "empty code",
			code:    "",
			wantErr: ErrInvalidLocale,
		},
		{
			name:    "malformed code",
			code:    "@@@",
			wantErr: ErrInvalidLocale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			locale, err := ParseLocale(tt.code)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("ParseLocale(%q) error = %v, want %v", tt.code, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLocale(%q) returned error: %v", tt.code, err)
			}
			if locale.Code != tt.wantCode {
				t.Errorf("ParseLocale(%q).Code = %q, want %q", tt.code, locale.Code, tt.wantCode)
			}
			if locale.Direction != tt.wantDirection {
				t.Errorf("ParseLocale(%q).Direction = %q, want %q", tt.code, locale.Direction, tt.wantDirection)
			}
		})
	}
}
