CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE bots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    hostname VARCHAR(255) NOT NULL,
    ip_address VARCHAR(45) NOT NULL,
    external_ip VARCHAR(45),
    os VARCHAR(100),
    architecture VARCHAR(50),
    username VARCHAR(100),
    privileged BOOLEAN DEFAULT FALSE,
    last_seen TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    status VARCHAR(50) DEFAULT 'active',
    beacon_interval INT DEFAULT 60,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE commands (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL DEFAULT 'shell',
    payload TEXT NOT NULL,
    output TEXT,
    status VARCHAR(50) DEFAULT 'pending',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    executed_at TIMESTAMP WITH TIME ZONE
);

CREATE TABLE credentials (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    username VARCHAR(255) NOT NULL,
    password VARCHAR(255) NOT NULL,
    target VARCHAR(255) NOT NULL,
    port INT DEFAULT 22,
    valid BOOLEAN DEFAULT FALSE,
    discovered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    bot_id UUID REFERENCES bots(id) ON DELETE SET NULL
);

CREATE TABLE campaigns (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(255) NOT NULL,
    description TEXT,
    type VARCHAR(50) NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    config JSONB,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE proxies (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    type VARCHAR(20) NOT NULL DEFAULT 'socks5',
    listen_port INT NOT NULL,
    status VARCHAR(50) DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE keylog (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    data TEXT NOT NULL,
    window_title VARCHAR(500),
    captured_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE TABLE exfil_data (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    bot_id UUID REFERENCES bots(id) ON DELETE CASCADE,
    type VARCHAR(50) NOT NULL,
    filename VARCHAR(500),
    data BYTEA,
    size BIGINT,
    uploaded_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE INDEX idx_bots_status ON bots(status);
CREATE INDEX idx_bots_last_seen ON bots(last_seen);
CREATE INDEX idx_commands_bot_id ON commands(bot_id);
CREATE INDEX idx_commands_status ON commands(status);
CREATE INDEX idx_credentials_target ON credentials(target);
CREATE INDEX idx_credentials_valid ON credentials(valid);
CREATE INDEX idx_proxies_bot_id ON proxies(bot_id);
