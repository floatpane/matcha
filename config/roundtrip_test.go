package config

import (
	"reflect"
	"testing"

	"github.com/zalando/go-keyring"
)

// fillAll sets every exported field of a struct to a non-zero value so a
// SaveConfig/LoadConfig round trip catches fields missing from the on-disk
// structs (secure mode used to drop mouse_enabled, body_cache_threshold_mb
// and undo_delay_seconds; per-account signatures were never loaded).
func fillAll(t *testing.T, v reflect.Value) {
	t.Helper()
	typ := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		name := typ.Field(i).Name
		if !typ.Field(i).IsExported() {
			continue
		}
		switch name {
		case "SC", "Accounts":
			continue
		case "PGPKeySource":
			f.SetString("yubikey")
			continue
		case "Protocol":
			f.SetString("imap")
			continue
		case "AuthMethod":
			f.SetString("password")
			continue
		case "PassCmd":
			continue // would be executed by LoadConfig
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString("v-" + name)
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(7)
		case reflect.Pointer:
			if f.Type().Elem().Kind() != reflect.Bool {
				t.Fatalf("fillAll: unsupported pointer field %s", name)
			}
			b := true
			f.Set(reflect.ValueOf(&b))
		case reflect.Slice:
			if f.Type() != reflect.TypeOf([]MailingList{}) {
				t.Fatalf("fillAll: unsupported slice field %s", name)
			}
			f.Set(reflect.ValueOf([]MailingList{{Name: "team", Addresses: []string{"a@example.com"}}}))
		case reflect.Map:
			if f.Type() != reflect.TypeOf(map[string]map[string]interface{}{}) {
				t.Fatalf("fillAll: unsupported map field %s", name)
			}
			f.Set(reflect.ValueOf(map[string]map[string]interface{}{"p": {"k": "v"}}))
		default:
			t.Fatalf("fillAll: unsupported field %s of kind %s; extend the test", name, f.Kind())
		}
	}
}

func compareAll(t *testing.T, want, got reflect.Value, prefix string) {
	t.Helper()
	typ := want.Type()
	for i := 0; i < want.NumField(); i++ {
		name := typ.Field(i).Name
		if !typ.Field(i).IsExported() || name == "SC" || name == "Accounts" {
			continue
		}
		if !reflect.DeepEqual(want.Field(i).Interface(), got.Field(i).Interface()) {
			t.Errorf("%s%s: saved %v, loaded %v", prefix, name, want.Field(i).Interface(), got.Field(i).Interface())
		}
	}
}

func TestSaveLoadConfigRoundTripsEveryField(t *testing.T) {
	for _, secure := range []bool{false, true} {
		name := "plain"
		if secure {
			name = "secure"
		}
		t.Run(name, func(t *testing.T) {
			isolateHome(t)
			keyring.MockInit()
			vaultMu.Lock()
			cachedVault = nil
			vaultMu.Unlock()
			t.Cleanup(func() {
				ClearSessionKey()
				vaultMu.Lock()
				cachedVault = nil
				vaultMu.Unlock()
			})

			cfg := &Config{}
			fillAll(t, reflect.ValueOf(cfg).Elem())
			acct := Account{ID: "acct-1", SC: &SessionCache{}}
			fillAll(t, reflect.ValueOf(&acct).Elem())
			cfg.Accounts = []Account{acct}

			if secure {
				if err := EnableSecureMode("correct horse battery", cfg); err != nil {
					t.Fatal(err)
				}
			} else if err := SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}

			got, err := LoadConfig()
			if err != nil {
				t.Fatal(err)
			}
			compareAll(t, reflect.ValueOf(cfg).Elem(), reflect.ValueOf(got).Elem(), "Config.")
			if len(got.Accounts) != 1 {
				t.Fatalf("accounts = %d", len(got.Accounts))
			}
			compareAll(t, reflect.ValueOf(acct), reflect.ValueOf(got.Accounts[0]), "Account.")
		})
	}
}
