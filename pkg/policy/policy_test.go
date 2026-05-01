package policy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicyAllowsOnlyLoopbackTargets(t *testing.T) {
	p := Default()

	for _, target := range []string{"127.0.0.1", "127.10.20.30", "::1", "localhost"} {
		if err := p.ValidateTarget(target); err != nil {
			t.Fatalf("default policy rejected loopback target %q: %v", target, err)
		}
	}

	for _, target := range []string{"8.8.8.8", "1.1.1.1", "example.com"} {
		if err := p.ValidateTarget(target); err == nil {
			t.Fatalf("default policy accepted external target %q", target)
		}
	}
}

func TestValidateTargetHonorsAllowAndBlockCIDRs(t *testing.T) {
	p := Policy{
		SafeMode:     true,
		AllowedCIDRs: []string{"10.0.0.0/8"},
		BlockedCIDRs: []string{"10.1.0.0/16"},
	}

	if err := p.ValidateTarget("10.2.3.4"); err != nil {
		t.Fatalf("allowed target rejected: %v", err)
	}
	if err := p.ValidateTarget("10.1.2.3"); err == nil {
		t.Fatal("blocked CIDR target accepted")
	}
	if err := p.ValidateTarget("192.168.1.10"); err == nil {
		t.Fatal("unlisted target accepted")
	}
	if err := p.ValidateTarget("10.2.0.0/24"); err != nil {
		t.Fatalf("allowed target CIDR rejected: %v", err)
	}
	if err := p.ValidateTarget("10.1.0.0/24"); err == nil {
		t.Fatal("blocked target CIDR accepted")
	}
	if err := p.ValidateTarget("192.168.1.0/24"); err == nil {
		t.Fatal("unlisted target CIDR accepted")
	}
}

func TestValidateTargetHonorsAllowedHosts(t *testing.T) {
	p := Policy{AllowedHosts: []string{"lab.internal", "target.local"}}

	if err := p.ValidateTarget("lab.internal"); err != nil {
		t.Fatalf("allowed host rejected: %v", err)
	}
	if err := p.ValidateTarget("other.internal"); err == nil {
		t.Fatal("unlisted host accepted")
	}
}

func TestValidateTargetRejectsMalformedTarget(t *testing.T) {
	p := Default()

	for _, target := range []string{"", " ", "bad host name", "http://127.0.0.1"} {
		if err := p.ValidateTarget(target); err == nil {
			t.Fatalf("malformed target %q accepted", target)
		}
	}
}

func TestLoadPolicyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.json")
	err := os.WriteFile(path, []byte(`{
		"safe_mode": true,
		"allow_external_brute": true,
		"allow_auto_deploy": false,
		"allowed_cidrs": ["10.0.0.0/8"],
		"allowed_hosts": ["lab.internal"],
		"blocked_cidrs": ["10.1.0.0/16"]
	}`), 0600)
	if err != nil {
		t.Fatalf("write policy file: %v", err)
	}

	p, err := LoadFile(path)
	if err != nil {
		t.Fatalf("load policy file: %v", err)
	}
	if !p.SafeMode || !p.AllowExternalBrute || p.AllowAutoDeploy {
		t.Fatalf("unexpected policy flags: %+v", p)
	}
	if err := p.ValidateTarget("10.2.3.4"); err != nil {
		t.Fatalf("loaded policy rejected allowed target: %v", err)
	}
}
