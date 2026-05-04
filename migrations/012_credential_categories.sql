ALTER TABLE credentials ALTER COLUMN password TYPE TEXT;
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS category VARCHAR(50) DEFAULT 'login';
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS campaign_id UUID REFERENCES campaigns(id) ON DELETE SET NULL;
ALTER TABLE credentials ADD COLUMN IF NOT EXISTS source_path VARCHAR(500);
DROP INDEX IF EXISTS idx_credentials_unique;
CREATE UNIQUE INDEX IF NOT EXISTS idx_credentials_unique ON credentials (username, target, port, service, category);
