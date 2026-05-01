-- Agent fingerprint for deduplication on re-registration
ALTER TABLE bots ADD COLUMN IF NOT EXISTS fingerprint VARCHAR(64);
CREATE UNIQUE INDEX IF NOT EXISTS idx_bots_fingerprint ON bots(fingerprint) WHERE fingerprint IS NOT NULL;

-- Kill date support
ALTER TABLE bots ADD COLUMN IF NOT EXISTS kill_date TIMESTAMP WITH TIME ZONE;

-- Performance indexes for campaign engine
CREATE INDEX IF NOT EXISTS idx_campaign_tasks_campaign_status ON campaign_tasks(campaign_id, status);
CREATE INDEX IF NOT EXISTS idx_commands_bot_status ON commands(bot_id, status);
CREATE INDEX IF NOT EXISTS idx_bots_status_lastseen ON bots(status, last_seen);
