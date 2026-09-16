package version

import "testing"

func TestCompare(t *testing.T) {
	for _, tc := range []struct {
		a, b  string
		order int
		ok    bool
	}{
		{"v0.1.1-beta2", "v0.1.1", -1, true},
		{"1.10.0", "1.9.0", 1, true},
		{"1.0.0-beta.10", "1.0.0-beta.2", 1, true},
		{"1.0.0-beta10", "1.0.0-beta2", -1, true}, // SemVer identifiers, not natural sort.
		{"1.0.0-2", "1.0.0-alpha", -1, true},
		{"1.0.0-beta", "1.0.0-beta.1", -1, true},
		{"v1.2.3+one", "1.2.3+two", 0, true},
		{" V1.2 ", "1.2.0", 0, true},
		{"1", "1.0.0", 0, true},
		{"1-beta", "1.0.0-beta", 0, true},
		{"1.2-rc.1+build", "1.2.0-rc.1", 0, true},
		{"999999999999999999999.0.0", "2.0.0", 1, true},
		{"2024-01-02", "1.0.0", 0, false},
		{"1.2.3.4", "1.2.3", 0, false},
		{"1.02.3", "1.2.3", 0, false},
		{"1.0.0-beta.01", "1.0.0", 0, false},
		{"1.0.0-", "1.0.0", 0, false},
		{"1.0.0+", "1.0.0", 0, false},
		{"release-final", "1.0.0", 0, false},
		{"", "1.0.0", 0, false},
	} {
		t.Run(tc.a+"_vs_"+tc.b, func(t *testing.T) {
			if order, ok := Compare(tc.a, tc.b); order != tc.order || ok != tc.ok {
				t.Fatalf("Compare = (%d, %v), want (%d, %v)", order, ok, tc.order, tc.ok)
			}
			if order, ok := Compare(tc.b, tc.a); order != -tc.order || ok != tc.ok {
				t.Fatalf("reverse Compare = (%d, %v)", order, ok)
			}
		})
	}
}
