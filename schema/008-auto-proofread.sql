-- SG Scout auto-proofreading schema extension (Constitution VII: user-owned schema).
-- Feature 005: ALTER proofread_card (source columns) + engine instance + auto run tables.
-- Execute with:  mysql -uroot sg_scout < schema/008-auto-proofread.sql
-- Kept in sync with model/proofread.go / model/proofread_engine.go / model/proofread_run.go (no AutoMigrate).
-- Design reference: specs/005-auto-proofreading/data-model.md.

-- 1) proofread_card: card provenance columns (manual cards keep defaults).
--    source: manual (004 existing rows) | engine; engine rows carry engine_name snapshot + run_id.
ALTER TABLE proofread_card
    ADD COLUMN source       VARCHAR(16)  NOT NULL DEFAULT 'manual' AFTER anchor_version,
    ADD COLUMN engine_name  VARCHAR(128) NOT NULL DEFAULT ''      AFTER source,
    ADD COLUMN run_id       BIGINT UNSIGNED NULL                  AFTER engine_name,
    ADD KEY idx_card_run (run_id);

-- 2) proofread_engine: one row per configurable engine instance (research D2).
--    config JSON holds non-secret type-specific settings (dict_path / provider+model).
--    Secrets (provider base_url/api_key) live in config.toml only (research D3).
CREATE TABLE IF NOT EXISTS proofread_engine (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    engine_type VARCHAR(16)     NOT NULL,                -- lexicon | llm | httpapi
    name        VARCHAR(128)    NOT NULL,                -- display name (cards snapshot it)
    enabled     TINYINT(1)      NOT NULL DEFAULT 0,      -- default off (FR-007)
    config      JSON            NOT NULL,                -- type-specific non-secret config
    note        VARCHAR(512)    NOT NULL DEFAULT '',
    created_at  DATETIME        NOT NULL,
    updated_at  DATETIME        NOT NULL,
    PRIMARY KEY (id),
    KEY idx_engine_enabled (enabled)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- 3) proofread_run: one row per auto-check execution (research D8/D14).
--    status: running | done | partial_failed | failed.
--    engines JSON: per-engine snapshot + result (name/type/config_summary/status/cards/dropped/cost_ms/error).
CREATE TABLE IF NOT EXISTS proofread_run (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    doc_id      BIGINT UNSIGNED NOT NULL,
    status      VARCHAR(16)     NOT NULL,                -- running | done | partial_failed | failed
    engines     JSON            NOT NULL,
    summary     VARCHAR(1024)   NOT NULL DEFAULT '',
    started_at  DATETIME        NOT NULL,
    finished_at DATETIME        NULL,
    PRIMARY KEY (id),
    KEY idx_run_doc_time (doc_id, id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
