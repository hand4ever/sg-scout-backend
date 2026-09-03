-- SG Scout proofread module schema (Constitution VII: user-owned schema).
-- Execute with:  mysql -uroot sg_scout < schema/007-proofread.sql
-- Kept in sync with model/proofread.go declarations (no AutoMigrate anywhere).
-- Design reference: specs/004-text-proofreading/data-model.md.

CREATE TABLE IF NOT EXISTS proofread_document (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    source_type   VARCHAR(16)     NOT NULL,                -- page | text | revision
    task_id       BIGINT UNSIGNED NULL,                    -- page: owning crawler task (snapshot, provenance only)
    page_id       BIGINT UNSIGNED NULL,                    -- page: bound crawler_page.id (1:1 via uk_doc_page)
    draft_version INT             NULL,                    -- page: bound page_version.version; NULL for text/revision
    parent_doc_id BIGINT UNSIGNED NULL,                    -- revision: parent proofread doc id (source chain)
    title         VARCHAR(1024)   NOT NULL DEFAULT '',
    source_url    VARCHAR(2048)   NOT NULL DEFAULT '',     -- page: page url snapshot at bind time
    draft_text    LONGTEXT        NOT NULL,                -- proofread draft as plain line stream snapshot
    created_at    DATETIME        NOT NULL,
    updated_at    DATETIME        NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_doc_page (page_id),
    KEY idx_doc_updated (updated_at),
    KEY idx_doc_parent (parent_doc_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS proofread_card (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    doc_id         BIGINT UNSIGNED NOT NULL,
    op_type        VARCHAR(8)      NOT NULL,               -- fix | replace | delete | insert
    start_line     INT             NOT NULL,               -- anchor start line (1-based)
    start_off      INT             NOT NULL,               -- anchor start offset (0-based, rune)
    end_line       INT             NOT NULL,               -- anchor end line (inclusive)
    end_off        INT             NOT NULL,               -- anchor end offset (exclusive; insert: empty point)
    orig_text      MEDIUMTEXT      NOT NULL,               -- selected original text snapshot
    replacement    MEDIUMTEXT      NOT NULL,                -- fix/replace/insert target; empty for delete (no DB default: TEXT columns)
    reason         VARCHAR(2000)   NOT NULL DEFAULT '',
    status         VARCHAR(16)     NOT NULL DEFAULT 'pending', -- pending | accepted | rejected
    reject_reason  VARCHAR(2000)   NOT NULL DEFAULT '',
    anchor_version INT             NULL,                   -- doc.draft_version at card creation (upgrade provenance)
    created_at     DATETIME        NOT NULL,
    updated_at     DATETIME        NOT NULL,
    PRIMARY KEY (id),
    KEY idx_card_doc_order (doc_id, start_line, start_off),
    KEY idx_card_status (doc_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS proofread_log (
    id         BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    doc_id     BIGINT UNSIGNED NOT NULL,
    action     VARCHAR(24)     NOT NULL,   -- card_create|card_update|card_delete|card_state|doc_upgrade
    card_id    BIGINT UNSIGNED NULL,
    summary    VARCHAR(1024)   NOT NULL,
    detail     JSON            NULL,
    created_at DATETIME        NOT NULL,
    PRIMARY KEY (id),
    KEY idx_log_doc_time (doc_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
