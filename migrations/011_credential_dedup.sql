CREATE UNIQUE INDEX IF NOT EXISTS idx_credentials_unique ON credentials (username, target, port, service);
