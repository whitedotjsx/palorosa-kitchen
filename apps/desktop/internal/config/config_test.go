package config

import "testing"

func TestPickPrefersSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		env      string
		want     string
	}{
		{"settings wins", "from-settings", "from-env", "from-settings"},
		{"env when settings empty", "", "from-env", "from-env"},
		{"empty when both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pick(tc.settings, tc.env); got != tc.want {
				t.Fatalf("pick(%q, %q) = %q, want %q", tc.settings, tc.env, got, tc.want)
			}
		})
	}
}
