package agent

import "testing"

func TestSOCKSTargetUsesJoinHostPortForIPv6(t *testing.T) {
	got := socksTarget("2001:db8::1", 443)
	want := "[2001:db8::1]:443"
	if got != want {
		t.Fatalf("target: got %q, want %q", got, want)
	}
}
