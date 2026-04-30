CREATE TABLE IF NOT EXISTS bot_tokens (
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    token VARCHAR(64) NOT NULL UNIQUE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (bot_id)
);

CREATE INDEX IF NOT EXISTS idx_bot_tokens_token ON bot_tokens(token);

ALTER TABLE credentials ADD COLUMN IF NOT EXISTS service VARCHAR(50) DEFAULT 'ssh';
