package keepawake

import "testing"

func TestIsCaffeinateComm(t *testing.T) {
	tests := []struct {
		out  string
		want bool
	}{
		{out: "/usr/bin/caffeinate\n", want: true},
		{out: "  caffeinate\n ", want: true},
		{out: "/usr/bin/caffeinated\n", want: false},
		{out: "/bin/sleep\n", want: false},
		{out: "", want: false},
	}
	for _, tc := range tests {
		if got := isCaffeinateComm(tc.out); got != tc.want {
			t.Errorf("isCaffeinateComm(%q) = %v, want %v", tc.out, got, tc.want)
		}
	}
}
