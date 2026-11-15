package fetcher

import (
	"testing"

	"github.com/floatpane/matcha/internal/testhome"
)

// TestMain keeps the tests away from the developer's real matcha config and
// caches.
func TestMain(m *testing.M) {
	testhome.Main(m)
}
