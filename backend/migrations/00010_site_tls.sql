-- +goose Up
CREATE TABLE site_tls_settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    credential_id INTEGER,
    enabled INTEGER NOT NULL DEFAULT 0,
    primary_domain TEXT NOT NULL DEFAULT '7g.chat',
    additional_domains_json TEXT NOT NULL DEFAULT '["www.7g.chat"]',
    status TEXT NOT NULL DEFAULT 'DISABLED',
    latest_certificate_id TEXT,
    latest_not_after DATETIME,
    staged_certificate_id TEXT,
    staged_at DATETIME,
    deployed_certificate_id TEXT,
    deployed_at DATETIME,
    last_checked_at DATETIME,
    last_error TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (credential_id) REFERENCES credentials(id) ON DELETE RESTRICT
);

INSERT INTO site_tls_settings (id) VALUES (1);

-- +goose Down
SELECT 'forward-only migration; restore SQLite backup for rollback';
