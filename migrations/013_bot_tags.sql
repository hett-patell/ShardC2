CREATE TABLE IF NOT EXISTS bot_tags (
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    tag VARCHAR(100) NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    PRIMARY KEY (bot_id, tag)
);

CREATE INDEX IF NOT EXISTS idx_bot_tags_tag ON bot_tags(tag);
CREATE INDEX IF NOT EXISTS idx_bot_tags_bot ON bot_tags(bot_id);
