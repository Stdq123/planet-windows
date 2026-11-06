package version

import "testing"

func TestCanonicalVersions(t *testing.T) {
	for _, text := range []string{"0.0.0", "4294967295.0.0", "0.0.1-alpha.1", "1.2.3-0", "1.2.3-alpha-01.X", "1.2.3-999999999999999999999999999999"} {
		if _, err := Parse(text); err != nil {
			t.Errorf("Parse(%q): %v", text, err)
		}
	}
	for _, text := range []string{"", "v1.2.3", "1.2", "1.2.3.4", "01.2.3", "1.02.3", "1.2.03", "4294967296.0.0", "-1.2.3", "1.2.3-", "1.2.3-alpha..1", "1.2.3-.alpha", "1.2.3-alpha.", "1.2.3-alpha.01", "1.2.3-00", "1.2.3-alpha_1", "1.2.3-测试", "1.2.3+build", "1.2.3-alpha+build", "1.2.3\n"} {
		if _, err := Parse(text); err == nil {
			t.Errorf("accepted invalid version %q", text)
		}
		if _, err := Compare(text, "1.2.3"); err == nil {
			t.Errorf("comparison accepted invalid first version %q", text)
		}
		if _, err := Compare("1.2.3", text); err == nil {
			t.Errorf("comparison accepted invalid second version %q", text)
		}
	}
}

func TestSemanticPrecedence(t *testing.T) {
	ordered := []string{"0.0.1-alpha", "0.0.1-alpha.1", "0.0.1-alpha.2", "0.0.1-alpha.10", "0.0.1-alpha.999999999999999999999999999999", "0.0.1-alpha.1000000000000000000000000000000", "0.0.1-alpha.beta", "0.0.1-beta", "0.0.1-beta.2", "0.0.1-beta.11", "0.0.1-rc.1", "0.0.1", "0.0.2-0", "0.1.0", "1.0.0"}
	for i, a := range ordered {
		for j, b := range ordered {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got, err := Compare(a, b); err != nil || got != want {
				t.Errorf("Compare(%q, %q) = %d, %v; want %d", a, b, got, err, want)
			}
		}
	}
}
