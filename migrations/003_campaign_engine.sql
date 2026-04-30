-- Campaign-to-bot assignment (many-to-many)
CREATE TABLE IF NOT EXISTS campaign_bots (
    campaign_id UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    assigned_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (campaign_id, bot_id)
);

-- Campaign tasks: tracks individual work items within a campaign
CREATE TABLE IF NOT EXISTS campaign_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    campaign_id UUID REFERENCES campaigns(id) ON DELETE CASCADE,
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    command_id UUID REFERENCES commands(id) ON DELETE SET NULL,
    task_name VARCHAR(255) NOT NULL,
    status VARCHAR(50) DEFAULT 'pending',
    output TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Link commands back to campaigns
ALTER TABLE commands ADD COLUMN IF NOT EXISTS campaign_id UUID REFERENCES campaigns(id) ON DELETE SET NULL;

-- Progress tracking on campaigns
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS total_tasks INT DEFAULT 0;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS completed_tasks INT DEFAULT 0;
ALTER TABLE campaigns ADD COLUMN IF NOT EXISTS failed_tasks INT DEFAULT 0;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_campaign_bots_campaign ON campaign_bots(campaign_id);
CREATE INDEX IF NOT EXISTS idx_campaign_bots_bot ON campaign_bots(bot_id);
CREATE INDEX IF NOT EXISTS idx_campaign_tasks_campaign ON campaign_tasks(campaign_id);
CREATE INDEX IF NOT EXISTS idx_campaign_tasks_status ON campaign_tasks(status);
CREATE INDEX IF NOT EXISTS idx_campaign_tasks_command ON campaign_tasks(command_id);
CREATE INDEX IF NOT EXISTS idx_commands_campaign ON commands(campaign_id);
