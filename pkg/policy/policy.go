package policy

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
)

type Policy struct {
	SafeMode           bool     `json:"safe_mode"`
	AllowExternalBrute bool     `json:"allow_external_brute"`
	AllowAutoDeploy    bool     `json:"allow_auto_deploy"`
	AllowedCIDRs       []string `json:"allowed_cidrs"`
	AllowedHosts       []string `json:"allowed_hosts"`
	BlockedCIDRs       []string `json:"blocked_cidrs"`
}

func Default() Policy {
	return Policy{
		SafeMode:           true,
		AllowExternalBrute: false,
		AllowAutoDeploy:    false,
		AllowedCIDRs:       []string{"127.0.0.0/8", "::1/128"},
		AllowedHosts:       []string{"localhost"},
	}
}

func LoadFile(path string) (Policy, error) {
	p := Default()
	if strings.TrimSpace(path) == "" {
		return p, nil
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, fmt.Errorf("read policy file: %w", err)
	}
	if err := json.Unmarshal(contents, &p); err != nil {
		return Policy{}, fmt.Errorf("parse policy file: %w", err)
	}
	if err := p.Validate(); err != nil {
		return Policy{}, err
	}
	return p, nil
}

func (p Policy) Validate() error {
	for _, cidr := range append([]string{}, append(p.AllowedCIDRs, p.BlockedCIDRs...)...) {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("invalid policy cidr %q: %w", cidr, err)
		}
	}
	for _, host := range p.AllowedHosts {
		if !isValidHostname(host) {
			return fmt.Errorf("invalid allowed host %q", host)
		}
	}
	return nil
}

func (p Policy) ValidateTarget(target string) error {
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("target is required")
	}
	if strings.Contains(target, "://") {
		return fmt.Errorf("target must be host or IP, got %q", target)
	}
	if _, targetNet, err := net.ParseCIDR(target); err == nil {
		return p.validateNetwork(targetNet)
	}

	if ip := net.ParseIP(target); ip != nil {
		return p.validateIP(ip)
	}

	if !isValidHostname(target) {
		return fmt.Errorf("invalid target host %q", target)
	}
	for _, allowed := range p.AllowedHosts {
		if strings.EqualFold(target, allowed) {
			return nil
		}
	}
	return fmt.Errorf("target host %q is outside allowed hosts", target)
}

func (p Policy) validateNetwork(target *net.IPNet) error {
	blocked, err := networkOverlapsCIDRs(target, p.BlockedCIDRs)
	if err != nil {
		return err
	}
	if blocked {
		return fmt.Errorf("target %s is blocked by policy", target)
	}

	for _, raw := range p.AllowedCIDRs {
		_, allowed, err := net.ParseCIDR(raw)
		if err != nil {
			return fmt.Errorf("invalid policy cidr %q: %w", raw, err)
		}
		if allowed.Contains(target.IP) && networkContainsLastIP(allowed, target) {
			return nil
		}
	}
	return fmt.Errorf("target %s is outside allowed CIDRs", target)
}

func (p Policy) validateIP(ip net.IP) error {
	blocked, err := ipInCIDRs(ip, p.BlockedCIDRs)
	if err != nil {
		return err
	}
	if blocked {
		return fmt.Errorf("target %s is blocked by policy", ip)
	}

	allowed, err := ipInCIDRs(ip, p.AllowedCIDRs)
	if err != nil {
		return err
	}
	if !allowed {
		return fmt.Errorf("target %s is outside allowed CIDRs", ip)
	}
	return nil
}

func ipInCIDRs(ip net.IP, cidrs []string) (bool, error) {
	for _, raw := range cidrs {
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return false, fmt.Errorf("invalid policy cidr %q: %w", raw, err)
		}
		if network.Contains(ip) {
			return true, nil
		}
	}
	return false, nil
}

func networkOverlapsCIDRs(target *net.IPNet, cidrs []string) (bool, error) {
	for _, raw := range cidrs {
		_, network, err := net.ParseCIDR(raw)
		if err != nil {
			return false, fmt.Errorf("invalid policy cidr %q: %w", raw, err)
		}
		if network.Contains(target.IP) || target.Contains(network.IP) {
			return true, nil
		}
	}
	return false, nil
}

func networkContainsLastIP(outer, inner *net.IPNet) bool {
	last := make(net.IP, len(inner.IP))
	copy(last, inner.IP)
	for i := range last {
		last[i] |= ^inner.Mask[i]
	}
	return outer.Contains(last)
}

func isValidHostname(host string) bool {
	if len(host) > 253 || strings.TrimSpace(host) != host || strings.Contains(host, " ") {
		return false
	}
	host = strings.TrimSuffix(host, ".")
	if host == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		if strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return false
		}
	}
	return true
}
