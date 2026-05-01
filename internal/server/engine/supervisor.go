package engine

import (
	"context"
	"fmt"
)

func (e *Engine) StartCampaignRun(ctx context.Context, campaignID, startedBy string) (string, error) {
	if e.db == nil {
		return "", fmt.Errorf("no database")
	}
	var runID string
	err := e.db.QueryRowContext(ctx, `
		INSERT INTO campaign_runs (campaign_id, started_by, status)
		VALUES ($1, $2, 'running') RETURNING id`,
		campaignID, startedBy,
	).Scan(&runID)
	if err != nil {
		return "", fmt.Errorf("create campaign run: %w", err)
	}
	return runID, nil
}

func (e *Engine) StopCampaignRun(ctx context.Context, runID, reason string) error {
	if e.db == nil {
		return fmt.Errorf("no database")
	}
	_, err := e.db.ExecContext(ctx, `
		UPDATE campaign_runs
		SET status = 'stopped', stopped_at = NOW(), stop_reason = $1
		WHERE id = $2 AND status = 'running'`,
		reason, runID,
	)
	return err
}

func (e *Engine) StopAllRunningRuns(ctx context.Context, reason string) error {
	if e.db == nil {
		return nil
	}
	_, err := e.db.ExecContext(ctx, `
		UPDATE campaign_runs
		SET status = 'stopped', stopped_at = NOW(), stop_reason = $1
		WHERE status = 'running'`,
		reason,
	)
	return err
}
