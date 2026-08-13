package resolver

import "testing"

func TestSM3Vectors(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "1ab21d8355cfa17f8e61194831e81a8f22bec8c728fefb747ed035eb5082aa2b"},
		{"abc", "66c7f0f462eeedd9d1f2d46bdc10e4e24167c4875cf2f7a2297da02b8f4ba8e0"},
		{"abcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcdabcd", "debe9ff92275b8a138604889c18e5a4d6fdb70e5387e5765293dcba39c0c5732"},
	}
	for _, c := range cases {
		if got := sm3Hex([]byte(c.in)); got != c.want {
			t.Errorf("sm3(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}
