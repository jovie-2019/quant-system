package mysqlstore

var schemaDDL = []string{
	`
CREATE TABLE IF NOT EXISTS orders (
	client_order_id VARCHAR(64) NOT NULL,
	venue_order_id VARCHAR(64) NOT NULL,
	symbol VARCHAR(32) NOT NULL,
	state VARCHAR(32) NOT NULL,
	filled_qty DECIMAL(32,16) NOT NULL,
	avg_price DECIMAL(32,16) NOT NULL,
	state_version BIGINT NOT NULL,
	updated_ms BIGINT NOT NULL,
	PRIMARY KEY (client_order_id),
	KEY idx_orders_symbol_state (symbol, state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
	`
CREATE TABLE IF NOT EXISTS positions (
	account_id VARCHAR(64) NOT NULL,
	symbol VARCHAR(32) NOT NULL,
	quantity DECIMAL(32,16) NOT NULL,
	avg_cost DECIMAL(32,16) NOT NULL,
	realized_pnl DECIMAL(32,16) NOT NULL,
	updated_ms BIGINT NOT NULL,
	PRIMARY KEY (account_id, symbol)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
	`
CREATE TABLE IF NOT EXISTS risk_decisions (
	intent_id VARCHAR(64) NOT NULL,
	strategy_id VARCHAR(64) NOT NULL,
	symbol VARCHAR(32) NOT NULL,
	side VARCHAR(8) NOT NULL,
	price DECIMAL(32,16) NOT NULL,
	quantity DECIMAL(32,16) NOT NULL,
	decision VARCHAR(16) NOT NULL,
	rule_id VARCHAR(128) NOT NULL,
	reason_code VARCHAR(128) NOT NULL,
	evaluated_ms BIGINT NOT NULL,
	updated_ms BIGINT NOT NULL,
	PRIMARY KEY (intent_id),
	KEY idx_risk_symbol_decision (symbol, decision)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_bin;
`,
}
