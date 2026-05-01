package engine

import (
	"context"
	"testing"

	"github.com/shardc2/shardc2/internal/database"
	"github.com/shardc2/shardc2/internal/testutil"
	"github.com/shardc2/shardc2/pkg/models"
	"github.com/shardc2/shardc2/pkg/policy"
)

func TestLaunchCreatesRunRecord(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupSupervisorTestData(t, db)

	var campaignID string
	if err := db.QueryRow(`
		INSERT INTO campaigns (name, type, status, config)
		VALUES ('run-record-test', $1, $2, '{}') RETURNING id`,
		models.CampaignTypeBrute, models.CampaignStatusCreated,
	).Scan(&campaignID); err != nil {
		t.Fatalf("insert campaign: %v", err)
	}

	e := NewWithPolicy(db, "", "", policy.Default())
	runID, err := e.StartCampaignRun(context.Background(), campaignID, "test-operator")
	if err != nil {
		t.Fatalf("start campaign run: %v", err)
	}
	if runID == "" {
		t.Fatal("run ID is empty")
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM campaign_runs WHERE id = $1`, runID).Scan(&status); err != nil {
		t.Fatalf("query run status: %v", err)
	}
	if status != "running" {
		t.Fatalf("run status: got %q, want %q", status, "running")
	}
}

func TestPauseCancelsRunningJob(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupSupervisorTestData(t, db)

	var campaignID string
	if err := db.QueryRow(`
		INSERT INTO campaigns (name, type, status, config)
		VALUES ('pause-run-test', $1, $2, '{}') RETURNING id`,
		models.CampaignTypeBrute, models.CampaignStatusCreated,
	).Scan(&campaignID); err != nil {
		t.Fatalf("insert campaign: %v", err)
	}

	e := NewWithPolicy(db, "", "", policy.Default())
	runID, err := e.StartCampaignRun(context.Background(), campaignID, "test-operator")
	if err != nil {
		t.Fatalf("start campaign run: %v", err)
	}

	if err := e.StopCampaignRun(context.Background(), runID, "operator_paused"); err != nil {
		t.Fatalf("stop campaign run: %v", err)
	}

	var status, stopReason string
	if err := db.QueryRow(`SELECT status, COALESCE(stop_reason, '') FROM campaign_runs WHERE id = $1`, runID).Scan(&status, &stopReason); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if status != "stopped" {
		t.Fatalf("run status: got %q, want %q", status, "stopped")
	}
	if stopReason != "operator_paused" {
		t.Fatalf("stop reason: got %q, want %q", stopReason, "operator_paused")
	}
}

func TestServerRestartLeavesRunsStopped(t *testing.T) {
	db := testutil.TestDB(t)
	cleanupSupervisorTestData(t, db)

	var campaignID string
	if err := db.QueryRow(`
		INSERT INTO campaigns (name, type, status, config)
		VALUES ('restart-run-test', $1, $2, '{}') RETURNING id`,
		models.CampaignTypeBrute, models.CampaignStatusRunning,
	).Scan(&campaignID); err != nil {
		t.Fatalf("insert campaign: %v", err)
	}

	db.Exec(`INSERT INTO campaign_runs (campaign_id, started_by, status) VALUES ($1, 'old-session', 'running')`, campaignID)

	e := NewWithPolicy(db, "", "", policy.Default())
	if err := e.PauseRunningCampaignsOnStartup(context.Background()); err != nil {
		t.Fatalf("startup pause: %v", err)
	}

	var runStatus string
	if err := db.QueryRow(`SELECT status FROM campaign_runs WHERE campaign_id = $1`, campaignID).Scan(&runStatus); err != nil {
		t.Fatalf("query run: %v", err)
	}
	if runStatus != "stopped" {
		t.Fatalf("run status after restart: got %q, want %q", runStatus, "stopped")
	}
}

func cleanupSupervisorTestData(t *testing.T, db *database.DB) {
	t.Helper()
	for _, q := range []string{
		`DELETE FROM campaign_runs`,
		`DELETE FROM campaign_tasks`,
		`DELETE FROM commands WHERE campaign_id IS NOT NULL`,
		`DELETE FROM campaigns WHERE name IN ('run-record-test', 'pause-run-test', 'restart-run-test')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("cleanup %q: %v", q, err)
		}
	}
}
