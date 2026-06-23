package updater

import (
	"testing"
)

func TestIsRC(t *testing.T) {
	cases := []struct {
		v    string
		want bool
	}{
		{"v1.0.0-rc1", true},
		{"1.0.0-rc2", true},
		{"v1.0.0", false},
		{"v0.9.0-rc1", false},
		{"v1.0.0-rc1-foo", true},
		{"dev", false},
	}
	for _, tc := range cases {
		if got := isRC(tc.v); got != tc.want {
			t.Errorf("isRC(%q) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

func TestRCNumber(t *testing.T) {
	cases := []struct {
		v    string
		want int
	}{
		{"v1.0.0-rc1", 1},
		{"1.0.0-rc12", 12},
		{"v1.0.0-rc0", 0},
		{"v1.0.0-rcfoo", 0},
		{"v1.0.0", 0},
	}
	for _, tc := range cases {
		if got := rcNumber(tc.v); got != tc.want {
			t.Errorf("rcNumber(%q) = %d, want %d", tc.v, got, tc.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, installed string
		want              bool
	}{
		{"v1.0.0-rc2", "v1.0.0-rc1", true},
		{"v1.0.0-rc1", "v1.0.0-rc2", false},
		{"v1.0.0-rc1", "v1.0.0-rc1", false},
		{"1.0.0-rc1", "1.0.0-rc2", false},
		{"1.0.0-rc2", "1.0.0-rc1", true},
		{"v1.0.0", "v1.0.0-rc3", true},
		{"v1.0.0-rc3", "v1.0.0", false},
		{"v1.0.1", "v1.0.0", true},
		{"v1.0.0", "v1.0.1", false},
		{"v2.0.0", "v1.0.0-rc3", true},
		{"dev", "v1.0.0-rc3", false},
	}
	for _, tc := range cases {
		if got := isNewer(tc.latest, tc.installed); got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v", tc.latest, tc.installed, got, tc.want)
		}
	}
}

func TestSemverLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"1.0.0-rc1", "1.0.0", true},
		{"1.0.0", "1.0.0-rc1", false},
		{"1.0.0", "1.0.1", true},
		{"1.0.1", "1.0.0", false},
		{"1.0.0-rc1", "1.0.0-rc2", true},
		{"1.0.0-rc2", "1.0.0-rc1", false},
		{"1.0.0", "1.0.0", false},
		{"2.0.0", "10.0.0", true},
	}
	for _, tc := range cases {
		if got := semverLess(tc.a, tc.b); got != tc.want {
			t.Errorf("semverLess(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestSplitPre(t *testing.T) {
	cases := []struct {
		v, core, pre string
	}{
		{"1.0.0-rc1", "1.0.0", "1"},
		{"1.0.0", "1.0.0", ""},
		{"1.0.0-alpha.2", "1.0.0", ".2"},
	}
	for _, tc := range cases {
		core, pre := splitPre(tc.v)
		if core != tc.core || pre != tc.pre {
			t.Errorf("splitPre(%q) = (%q, %q), want (%q, %q)", tc.v, core, pre, tc.core, tc.pre)
		}
	}
}
