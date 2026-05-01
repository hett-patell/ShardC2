package engine

import (
	"context"
	"testing"

	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/testutil"
	"github.com/shardc2/shardc2/pkg/models"
	"github.com/shardc2/shardc2/pkg/policy"
)

func TestSafeModePausesRunningCampaignsOnStartup(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupEngineTestData(t, db)

	var campaignID string
	if err := db.QueryRow(`
		INSERT INTO campaigns (name, type, status, config)
		VALUES ('safe-mode-pause-test', $1, $2, '{}') RETURNING id`,
		models.CampaignTypeRecon, models.CampaignStatusRunning,
	).Scan(&campaignID); err != nil {
		t.Fatalf("insert running campaign: %v", err)
	}

	e := NewWithPolicy(db, "", "", policy.Default())
	if err := e.PauseRunningCampaignsOnStartup(context.Background()); err != nil {
		t.Fatalf("pause running campaigns: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM campaigns WHERE id = $1`, campaignID).Scan(&status); err != nil {
		t.Fatalf("query campaign status: %v", err)
	}
	if status != models.CampaignStatusPaused {
		t.Fatalf("status: got %q, want %q", status, models.CampaignStatusPaused)
	}
}

func TestSafeModeDisabledDoesNotPauseRunningCampaignsOnStartup(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupEngineTestData(t, db)

	var campaignID string
	if err := db.QueryRow(`
		INSERT INTO campaigns (name, type, status, config)
		VALUES ('safe-mode-disabled-test', $1, $2, '{}') RETURNING id`,
		models.CampaignTypeRecon, models.CampaignStatusRunning,
	).Scan(&campaignID); err != nil {
		t.Fatalf("insert running campaign: %v", err)
	}

	p := policy.Default()
	p.SafeMode = false
	e := NewWithPolicy(db, "", "", p)
	if err := e.PauseRunningCampaignsOnStartup(context.Background()); err != nil {
		t.Fatalf("pause running campaigns: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM campaigns WHERE id = $1`, campaignID).Scan(&status); err != nil {
		t.Fatalf("query campaign status: %v", err)
	}
	if status != models.CampaignStatusRunning {
		t.Fatalf("status: got %q, want %q", status, models.CampaignStatusRunning)
	}
}

func TestValidateBrutePolicyRejectsTargetsOutsidePolicy(t *testing.T) {
	e := NewWithPolicy(nil, "", "", policy.Default())
	err := e.validateBrutePolicy(bruteConfig{Targets: []string{"8.8.8.8"}})
	if err == nil {
		t.Fatal("expected external target to be rejected")
	}
}

func TestValidateBrutePolicyRequiresExternalBrutePolicy(t *testing.T) {
	e := NewWithPolicy(nil, "", "", policy.Default())
	err := e.validateBrutePolicy(bruteConfig{Mode: "external", Targets: []string{"127.0.0.1"}})
	if err == nil {
		t.Fatal("expected external mode to be rejected")
	}

	p := policy.Default()
	p.AllowExternalBrute = true
	e = NewWithPolicy(nil, "", "", p)
	if err := e.validateBrutePolicy(bruteConfig{Mode: "external", Targets: []string{"127.0.0.1"}}); err != nil {
		t.Fatalf("expected explicit external policy to pass: %v", err)
	}
}

func cleanupEngineTestData(t *testing.T, db *database.DB) {
	t.Helper()
	if _, err := db.Exec(`DELETE FROM campaigns WHERE name IN ('safe-mode-pause-test', 'safe-mode-disabled-test')`); err != nil {
		t.Fatalf("cleanup campaigns: %v", err)
	}
}
