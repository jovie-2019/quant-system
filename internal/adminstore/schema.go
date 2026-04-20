package adminstore

var adminDDL = []string{
	`
CREATE TABLE IF NOT EXISTS exchanges (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	name VARCHAR(64) NOT NULL UNIQUE,
	venue VARCHAR(32) NOT NULL,
	status VARCHAR(16) NOT NULL DEFAULT 'active',
	created_ms BIGINT NOT NULL,
	updated_ms BIGINT NOT NULL
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
	`
CREATE TABLE IF NOT EXISTS api_keys (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	exchange_id BIGINT NOT NULL,
	label VARCHAR(64) NOT NULL,
	api_key_enc TEXT NOT NULL,
	api_secret_enc TEXT NOT NULL,
	passphrase_enc TEXT NOT NULL,
	permissions VARCHAR(128) NOT NULL DEFAULT 'trade,read',
	status VARCHAR(16) NOT NULL DEFAULT 'active',
	created_ms BIGINT NOT NULL,
	updated_ms BIGINT NOT NULL,
	FOREIGN KEY (exchange_id) REFERENCES exchanges(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
	`
CREATE TABLE IF NOT EXISTS strategy_configs (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	strategy_id VARCHAR(64) NOT NULL UNIQUE,
	strategy_type VARCHAR(64) NOT NULL,
	exchange_id BIGINT NOT NULL,
	api_key_id BIGINT NOT NULL,
	config_json TEXT NOT NULL,
	status VARCHAR(16) NOT NULL DEFAULT 'stopped',
	created_ms BIGINT NOT NULL,
	updated_ms BIGINT NOT NULL,
	FOREIGN KEY (exchange_id) REFERENCES exchanges(id),
	FOREIGN KEY (api_key_id) REFERENCES api_keys(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
	`
CREATE TABLE IF NOT EXISTS strategy_param_revisions (
	id BIGINT AUTO_INCREMENT PRIMARY KEY,
	strategy_id VARCHAR(64) NOT NULL,
	revision BIGINT NOT NULL,
	command_type VARCHAR(32) NOT NULL,
	params_before TEXT,
	params_after TEXT,
	actor VARCHAR(128) NOT NULL,
	reason TEXT,
	issued_ms BIGINT NOT NULL,
	ack_received_ms BIGINT,
	ack_accepted TINYINT(1),
	ack_error TEXT,
	UNIQUE KEY uk_strategy_revision (strategy_id, revision),
	KEY idx_strategy_issued (strategy_id, issued_ms)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
}
