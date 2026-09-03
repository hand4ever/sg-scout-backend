-- Feature 002 extension: task-level external-host whitelist (allow_hosts).
-- Lets a crawl follow links onto named external hosts (e.g. mp.weixin.qq.com
-- articles linked from rjh.com.cn list pages). Whitelisted pages are fetched
-- once and never expanded (leaf). Empty string = same-site-only behaviour.
-- Executed by user/operator (Constitution VII). Mirrored into crawler.sql.

ALTER TABLE crawler_task
    ADD COLUMN allow_hosts VARCHAR(512) NOT NULL DEFAULT '' AFTER include_subdomain;
