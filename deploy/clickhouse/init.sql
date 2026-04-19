-- Schema for the quant-system time-series store.
-- Loaded once on first ClickHouse container boot via the official
-- /docker-entrypoint-initdb.d mechanism.
--
-- ReplacingMergeTree deduplicates rows with the same ORDER BY key during
-- background merges; querying with FINAL forces the engine to dedupe on
-- read, which keeps Upsert idempotent even before compaction has run.

CREATE DATABASE IF NOT EXISTS quant;

CREATE TABLE IF NOT EXISTS quant.klines (
    venue        LowCardinality(String),
    symbol       LowCardinality(String),
    interval     LowCardinality(String),
    open_time    DateTime64(3, 'UTC'),
    close_time   DateTime64(3, 'UTC'),
    open         Float64,
    high         Float64,
    low          Float64,
    close        Float64,
    volume       Float64,
    ingested_at  DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(open_time)
ORDER BY (venue, symbol, interval, open_time);

-- Secondary bloom-filter index on symbol accelerates "give me a year of
-- BTCUSDT" style queries that span many partitions.
ALTER TABLE quant.klines
    ADD INDEX IF NOT EXISTS idx_symbol (symbol)
    TYPE bloom_filter(0.01) GRANULARITY 1;

-- Placeholder for regime_history — written by the future regime-detector
-- service. Kept here so schema migrations live in one place.
CREATE TABLE IF NOT EXISTS quant.regime_history (
    venue        LowCardinality(String),
    symbol       LowCardinality(String),
    interval     LowCardinality(String),
    bar_time     DateTime64(3, 'UTC'),
    method       LowCardinality(String),  -- 'threshold' | 'gmm' | 'hmm'
    regime       LowCardinality(String),  -- 'trend_up' | 'trend_down' | 'range' | 'high_vol' | 'low_liq'
    confidence   Float32,
    adx          Float32,
    atr          Float32,
    bbw          Float32,
    hurst        Float32,
    ingested_at  DateTime DEFAULT now()
) ENGINE = ReplacingMergeTree(ingested_at)
PARTITION BY toYYYYMM(bar_time)
ORDER BY (venue, symbol, interval, bar_time, method);
