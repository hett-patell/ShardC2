CREATE TABLE IF NOT EXISTS agent_builds (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    goos VARCHAR(20) NOT NULL,
    goarch VARCHAR(20) NOT NULL,
    profile VARCHAR(100) NOT NULL DEFAULT 'default',
    payload_key_fingerprint VARCHAR(128),
    requested_by VARCHAR(100),
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    artifact_path TEXT,
    error_message TEXT,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX IF NOT EXISTS idx_agent_builds_status ON agent_builds(status);
CREATE INDEX IF NOT EXISTS idx_agent_builds_created_at ON agent_builds(created_at);
