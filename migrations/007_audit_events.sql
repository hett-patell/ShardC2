CREATE TABLE IF NOT EXISTS audit_events (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    operator_id UUID,
    operator_username VARCHAR(100),
    operator_role VARCHAR(50),
    source_ip INET,
    action VARCHAR(100) NOT NULL,
    object_type VARCHAR(100),
    object_id TEXT,
    outcome VARCHAR(50) NOT NULL,
    details JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_events_created_at ON audit_events(created_at);
CREATE INDEX IF NOT EXISTS idx_audit_events_operator ON audit_events(operator_id);
CREATE INDEX IF NOT EXISTS idx_audit_events_action ON audit_events(action);
