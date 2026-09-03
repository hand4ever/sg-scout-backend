-- Feature 002: task-level content_mode (main | full).
-- main (default) = archive article-only body via go-readability extraction on
-- the goquery engine; full = whole-page markdown (legacy behaviour).
-- Mirrored into crawler.sql.

ALTER TABLE crawler_task
    ADD COLUMN content_mode VARCHAR(16) NOT NULL DEFAULT 'main' AFTER include_url;
