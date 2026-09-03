-- Feature 002 extension: task-level robots.txt opt-out.
-- API surfaces respect_robots (default true = robots respected); the column
-- stores the inverse ignore_robots so that the common default (false/respect)
-- is the zero value — GORM skips zero-value fields on Create, so only an
-- explicit opt-out writes a row value. WeChat article paths
-- (mp.weixin.qq.com/s/...) are robots-disallowed yet are real target content
-- when crawling hospital media lists; opting out is per-task and explicit.
-- Mirrored into crawler.sql.
--
-- NOTE (fresh-DB deploy): this file is an incremental CHANGE for databases
-- that already ran the earlier respect_robots variant of this migration.
-- On a fresh database execute instead:
--   ALTER TABLE crawler_task
--       ADD COLUMN ignore_robots TINYINT(1) NOT NULL DEFAULT 0 AFTER allow_hosts;

ALTER TABLE crawler_task
    CHANGE COLUMN respect_robots ignore_robots TINYINT(1) NOT NULL DEFAULT 0;
