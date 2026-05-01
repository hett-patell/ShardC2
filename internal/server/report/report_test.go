package report

import (
	"strings"
	"testing"

	"github.com/shardc2/shardc2/internal/testutil"
	"github.com/shardc2/shardc2/pkg/models"
)

func TestGenerateCampaignReportContainsExpectedSections(t *testing.T) {
	db := testutil.TestDB(t)

	var campaignID string
	if err := db.QueryRow(`
		INSERT INTO campaigns (name, type, status, config)
		VALUES ('report-test', $1, $2, '{"targets":["127.0.0.1"]}') RETURNING id`,
		models.CampaignTypeBrute, models.CampaignStatusCompleted,
	).Scan(&campaignID); err != nil {
		t.Fatalf("insert campaign: %v", err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM audit_events WHERE object_id = $1`, campaignID)
		db.Exec(`DELETE FROM commands WHERE campaign_id = $1`, campaignID)
		db.Exec(`DELETE FROM campaigns WHERE id = $1`, campaignID)
	})

	db.Exec(`INSERT INTO audit_events (operator_username, operator_role, source_ip, action, object_type, object_id, outcome) VALUES ('admin', 'admin', '127.0.0.1', 'campaign.create', 'campaign', $1, 'success')`, campaignID)

	md, err := GenerateCampaignReport(db, campaignID)
	if err != nil {
		t.Fatalf("generate report: %v", err)
	}

	for _, section := range []string{"# Campaign Report: report-test", "## Configuration", "## Command Summary", "## Audit Trail", "campaign.create"} {
		if !strings.Contains(md, section) {
			t.Fatalf("report missing section %q:\n%s", section, md)
		}
	}
}

func TestGenerateCampaignReportFailsForMissingCampaign(t *testing.T) {
	db := testutil.TestDB(t)

	_, err := GenerateCampaignReport(db, "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Fatal("expected error for missing campaign")
	}
}
