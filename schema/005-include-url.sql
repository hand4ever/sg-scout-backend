-- Feature 002 extension: task-level include_url filter (only discovered links
-- containing one of the comma-separated URL substrings are followed; the entry
-- URL is always fetched). Use case: hospital list pages whose article detail
-- links share a path pattern (e.g. yydtxs.asp) while the same-site nav would
-- otherwise pull whole-site column pages. Mirrored into crawler.sql.

ALTER TABLE crawler_task
    ADD COLUMN include_url VARCHAR(1024) NOT NULL DEFAULT '' AFTER ignore_robots;
