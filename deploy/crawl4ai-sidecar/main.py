"""Crawl4AI sidecar for SG Scout (feature 002, research.md §5 upgrade path).

Thin local FastAPI service exposing the sync single-page crawl contract the Go
backend adapter expects (POST /crawl + GET /health on 127.0.0.1:11235), backed
by the Crawl4AI SDK (full browser rendering, no HTTP-API restrictions).

Why not the official server: v0.9.x official server ships as a Docker image
(pip package has no server command) and this machine's docker registry mirror
rejects the image; the documented fallback is a self-maintained SDK wrapper.

Run:  .venv/bin/uvicorn main:app --host 127.0.0.1 --port 11235
"""

from __future__ import annotations

import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI, Request

from crawl4ai import AsyncWebCrawler, BrowserConfig, CacheMode, CrawlerRunConfig

log = logging.getLogger("crawl4ai-sidecar")

crawler: AsyncWebCrawler | None = None
_lock = asyncio.Lock()


def _attr(obj, name, default=None):
    return getattr(obj, name, default) if obj is not None else default


def _md_raw(markdown_obj):
    if markdown_obj is None:
        return ""
    if isinstance(markdown_obj, dict):
        return markdown_obj.get("raw_markdown", "") or ""
    return getattr(markdown_obj, "raw_markdown", "") or ""


def _meta_title(result):
    m = _attr(result, "metadata", None)
    if isinstance(m, dict):
        return m.get("title", "") or ""
    return _attr(m, "title", "") or ""


@asynccontextmanager
async def lifespan(_app: FastAPI):
    global crawler
    crawler = AsyncWebCrawler(config=BrowserConfig(headless=True, browser_type="chromium"))
    await crawler.start()
    yield
    await crawler.close()


app = FastAPI(title="crawl4ai-sidecar", lifespan=lifespan)


@app.get("/health")
async def health():
    return {"status": "ok"}


@app.post("/crawl")
async def crawl(request: Request):
    global crawler
    body = await request.json()
    urls = body.get("urls") or []
    cc = body.get("crawler_config") or {}
    page_timeout = int(cc.get("page_timeout") or 60000)

    if not isinstance(urls, list) or not urls:
        return {"success": False, "results": [], "error": "urls required"}
    if crawler is None:
        return {"success": False, "results": [], "error": "crawler not started"}

    cfg = CrawlerRunConfig(
        cache_mode=CacheMode.BYPASS,
        page_timeout=page_timeout,
        check_robots_txt=False,
    )
    results = []
    async with _lock:  # one browser crawl at a time (throttle is caller-side)
        for raw in urls[:10]:
            item = {"url": raw, "success": False, "status_code": None,
                    "redirected_status_code": None, "redirected_url": "",
                    "metadata": {"title": ""}, "html": "", "markdown": None,
                    "error_message": ""}
            try:
                r = await crawler.arun(str(raw), config=cfg)
                item["success"] = bool(_attr(r, "success", False))
                item["url"] = _attr(r, "url") or raw
                item["status_code"] = _attr(r, "status_code")
                item["redirected_status_code"] = _attr(r, "redirected_status_code")
                item["redirected_url"] = _attr(r, "redirected_url") or ""
                item["metadata"] = {"title": _meta_title(r)}
                item["html"] = _attr(r, "html", "") or ""
                md = _attr(r, "markdown", None)
                raw_md = _md_raw(md)
                if raw_md:
                    item["markdown"] = {"raw_markdown": raw_md}
                item["error_message"] = _attr(r, "error_message") or ""
            except Exception as e:  # per-page failure must not kill the batch
                log.warning("crawl %s failed: %s", raw, e)
                item["error_message"] = str(e)[:300]
            results.append(item)
    return {"success": True, "results": results}
