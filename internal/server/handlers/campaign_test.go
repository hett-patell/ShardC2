package handlers

import (
	"testing"

	"github.com/shardc2/shardc2/pkg/models"
	"github.com/shardc2/shardc2/pkg/policy"
)

func TestValidateCampaignConfigBlocksBruteTargetsOutsidePolicy(t *testing.T) {
	err := ValidateCampaignConfig(policy.Default(), models.CampaignTypeBrute, `{"targets":["8.8.8.8"],"ports":[22]}`)
	if err == nil {
		t.Fatal("expected public brute target to be blocked")
	}
}

func TestValidateCampaignConfigAllowsScopedBruteTargets(t *testing.T) {
	err := ValidateCampaignConfig(policy.Default(), models.CampaignTypeBrute, `{"targets":["127.0.0.1","localhost"],"ports":[22]}`)
	if err != nil {
		t.Fatalf("expected scoped brute targets to pass: %v", err)
	}
}

func TestValidateCampaignConfigRequiresExternalBrutePolicy(t *testing.T) {
	config := `{"mode":"external","targets":["127.0.0.1"],"ports":[22]}`
	if err := ValidateCampaignConfig(policy.Default(), models.CampaignTypeBrute, config); err == nil {
		t.Fatal("expected external brute to require explicit policy")
	}

	p := policy.Default()
	p.AllowExternalBrute = true
	if err := ValidateCampaignConfig(p, models.CampaignTypeBrute, config); err != nil {
		t.Fatalf("expected explicit external brute policy to pass: %v", err)
	}
}

func TestValidateCampaignConfigSkipsTargetValidationForAssignedBotCampaigns(t *testing.T) {
	for _, campaignType := range []string{models.CampaignTypeExfil, models.CampaignTypePersist, models.CampaignTypeCustom} {
		t.Run(campaignType, func(t *testing.T) {
			err := ValidateCampaignConfig(policy.Default(), campaignType, `{"targets":["8.8.8.8"]}`)
			if err != nil {
				t.Fatalf("expected %s config to skip external target validation: %v", campaignType, err)
			}
		})
	}
}

func TestDryRunValidationReturnsTargetAndPolicyInfo(t *testing.T) {
	p := policy.Default()

	result := DryRunValidate(p, models.CampaignTypeBrute, `{"targets":["127.0.0.1","8.8.8.8"],"ports":[22]}`)
	if result.TotalTargets != 2 {
		t.Fatalf("total targets: got %d, want 2", result.TotalTargets)
	}
	if result.BlockedTargets != 1 {
		t.Fatalf("blocked targets: got %d, want 1", result.BlockedTargets)
	}
	if len(result.PolicyWarnings) == 0 {
		t.Fatal("expected policy warnings for blocked target")
	}
}

func TestDryRunValidationPassesCleanConfig(t *testing.T) {
	p := policy.Default()

	result := DryRunValidate(p, models.CampaignTypeBrute, `{"targets":["127.0.0.1"],"ports":[22]}`)
	if result.BlockedTargets != 0 {
		t.Fatalf("blocked targets: got %d, want 0", result.BlockedTargets)
	}
	if len(result.PolicyWarnings) != 0 {
		t.Fatalf("unexpected policy warnings: %v", result.PolicyWarnings)
	}
}

func TestDryRunValidationForNonBruteCampaigns(t *testing.T) {
	p := policy.Default()
	result := DryRunValidate(p, models.CampaignTypeRecon, `{}`)
	if result.BlockedTargets != 0 {
		t.Fatalf("blocked targets for recon: got %d, want 0", result.BlockedTargets)
	}
}
