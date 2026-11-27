package setcategory

import "testing"

func TestStatusText(t *testing.T) {
	for _, tc := range []struct {
		category string
		o        Opts
		want     string
	}{
		{"tools/editor", Opts{}, "category tools/editor"},
		{"", Opts{}, "category cleared"},
		{"", Opts{Inherits: true}, "category cleared"},
		{"", Opts{Inherits: true, Inherited: "art"}, "using global category art"},
		{"own", Opts{Inherits: true, Inherited: "art"}, "category own"},
	} {
		if got := statusText(tc.category, tc.o); got != tc.want {
			t.Errorf("statusText(%q, %+v) = %q, want %q", tc.category, tc.o, got, tc.want)
		}
	}
}
