-- SG Scout crawler module schema (Constitution VII: user-owned schema).
-- Execute with:  mysql -uroot sg_scout < schema/crawler.sql
-- Kept in sync with model/crawler.go declarations (no AutoMigrate anywhere).

CREATE TABLE IF NOT EXISTS crawler_task (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    source_type         VARCHAR(32)     NOT NULL DEFAULT 'web',
    entry_url           VARCHAR(2048)   NOT NULL,
    entry_url_key       CHAR(64)        NOT NULL,
    depth               INT             NOT NULL DEFAULT 0,
    include_subdomain   TINYINT(1)      NOT NULL DEFAULT 0,
    page_limit          INT             NOT NULL DEFAULT 10,
    retry_times         INT             NOT NULL DEFAULT 3,
    retry_interval_s    INT             NOT NULL DEFAULT 2,
    throttle_pages      INT             NOT NULL DEFAULT 100,
    throttle_seconds    INT             NOT NULL DEFAULT 60,
    timeout_s           INT             NOT NULL DEFAULT 600,
    status              VARCHAR(16)     NOT NULL DEFAULT 'pending',
    page_count          INT             NOT NULL DEFAULT 0,
    last_run_at         DATETIME        NULL,
    created_at          DATETIME        NOT NULL,
    updated_at          DATETIME        NOT NULL,
    PRIMARY KEY (id),
    KEY idx_task_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS crawler_run (
    id            BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    task_id       BIGINT UNSIGNED NOT NULL,
    kind          VARCHAR(8)      NOT NULL,
    engine        VARCHAR(16)     NOT NULL DEFAULT 'firecrawl',
    job_id        VARCHAR(64)     NULL,
    status        VARCHAR(16)     NOT NULL DEFAULT 'queued',
    started_at    DATETIME        NULL,
    finished_at   DATETIME        NULL,
    total_found   INT             NOT NULL DEFAULT 0,
    page_new      INT             NOT NULL DEFAULT 0,
    page_changed  INT             NOT NULL DEFAULT 0,
    page_offline  INT             NOT NULL DEFAULT 0,
    page_failed   INT             NOT NULL DEFAULT 0,
    error_msg     VARCHAR(1024)   NULL,
    created_at    DATETIME        NOT NULL,
    PRIMARY KEY (id),
    KEY idx_run_task_created (task_id, created_at),
    KEY idx_run_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS crawler_page (
    id                  BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    task_id             BIGINT UNSIGNED NOT NULL,
    url                 VARCHAR(2048)   NOT NULL,
    url_key             CHAR(64)        NOT NULL,
    depth               INT             NOT NULL DEFAULT 0,
    title               VARCHAR(1024)   NOT NULL DEFAULT '',
    latest_version      INT             NOT NULL DEFAULT 0,
    latest_fingerprint  CHAR(64)        NOT NULL DEFAULT '',
    first_seen_at       DATETIME        NOT NULL,
    last_seen_at        DATETIME        NOT NULL,
    created_at          DATETIME        NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_task_urlkey (task_id, url_key),
    KEY idx_page_task (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS page_version (
    id           BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    page_id      BIGINT UNSIGNED NOT NULL,
    version      INT             NOT NULL,
    run_id       BIGINT UNSIGNED NOT NULL,
    kind         VARCHAR(8)      NOT NULL,
    fingerprint  CHAR(64)        NOT NULL,
    char_count   INT             NOT NULL DEFAULT 0,
    crawled_at   DATETIME        NOT NULL,
    created_at   DATETIME        NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_page_version (page_id, version),
    KEY idx_version_run (run_id),
    KEY idx_version_page_time (page_id, crawled_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS run_page (
    id          BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    run_id      BIGINT UNSIGNED NOT NULL,
    page_id     BIGINT UNSIGNED NOT NULL,
    status      VARCHAR(16)     NOT NULL,
    error       VARCHAR(512)    NULL,
    crawled_at  DATETIME        NULL,
    created_at  DATETIME        NOT NULL,
    PRIMARY KEY (id),
    UNIQUE KEY uk_run_page (run_id, page_id),
    KEY idx_runpage_status (run_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
