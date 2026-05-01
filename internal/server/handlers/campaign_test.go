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
