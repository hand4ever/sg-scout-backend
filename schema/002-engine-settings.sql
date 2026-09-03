-- SG Scout feature 002 migration: task engine snapshot + system settings tables.
-- Constitution VII: user-owned schema. Direct execution authorized by user 2026-09-03.
-- Execute: mysql -uroot sg_scout < schema/002-engine-settings.sql
-- Kept in sync with model/crawler.go + model/settings.go declarations (no AutoMigrate).

-- 1) Task-level engine snapshot column (spec 002 FR-003: archived & locked at creation).
ALTER TABLE crawler_task
    ADD COLUMN engine VARCHAR(16) NOT NULL DEFAULT 'goquery' AFTER source_type;

-- 2) Existing 001 tasks were all executed by the firecrawl engine (run.engine=firecrawl);
--    rewrite their snapshot so fingerprint scope stays consistent for future checks.
UPDATE crawler_task SET engine = 'firecrawl';

-- 3) Run default follows the new task default (CreateRun snapshots task.Engine anyway).
ALTER TABLE crawler_run
    MODIFY COLUMN engine VARCHAR(16) NOT NULL DEFAULT 'goquery';

-- 4) System settings (runtime config source replacing config.toml defaults; data-model.md §3).
CREATE TABLE IF NOT EXISTS system_settings (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    skey        VARCHAR(64)     NOT NULL,
    svalue      JSON            NOT NULL,
    note        VARCHAR(512)    NULL,
    created_at  DATETIME        NOT NULL,
    updated_at  DATETIME        NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_settings_key (skey)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 5) Settings change log (data-model.md §4: audit, reset, rollback support).
CREATE TABLE IF NOT EXISTS system_settings_log (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    skey        VARCHAR(64)     NOT NULL,
    old_value   JSON            NULL,
    new_value   JSON            NOT NULL,
    note        VARCHAR(512)    NULL,
    created_at  DATETIME        NOT NULL,
    PRIMARY KEY (id),
    KEY idx_settings_log_key_time (skey, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
