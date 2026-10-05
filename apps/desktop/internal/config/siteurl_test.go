package config

import "testing"

func TestSiteURL(t *testing.T) {
	cases := []struct{ raw, domain, want string }{
		{"", "palorosabreakfast.com", "https://palorosabreakfast.com"},
		{"", "https://tienda.com/", "https://tienda.com"},
		{"https://palorosabreakfast.com/wp-admin/", "x.com", "https://palorosabreakfast.com"},
		{"palorosabreakfast.com/wp-login.php", "", "https://palorosabreakfast.com"},
		{"http://localhost:8080/blog", "", "http://localhost:8080/blog"},
		{"", "", "https://" + defaultDomain},
	}
	for _, c := range cases {
		if got := SiteURL(c.raw, c.domain); got != c.want {
			t.Errorf("SiteURL(%q, %q) = %q, want %q", c.raw, c.domain, got, c.want)
		}
	}
}
