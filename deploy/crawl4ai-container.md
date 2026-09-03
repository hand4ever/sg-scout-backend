# Crawl4AI runtime: pip sidecar (feature 002, research.md §5)

SG Scout 抓取引擎 `crawl4ai` = 本地浏览器渲染抓取服务。**首选形态 = 自维护 Python
sidecar**（官方 Docker server 的替代：v0.9.x pip 包无 server 命令；本机 docker
registry 镜像源拒拉官方镜像，走 research.md §5 预留的 SDK 薄封装路径）。
Go 适配器只调同步 `POST /crawl` 单页（深度任务由后端本地 BFS 驱动器逐页驱动），
sidecar 契约与官方 HTTP 面一致，适配器零差异。

## 首次安装（一次性）

```bash
cd sg-scout-backend/deploy/crawl4ai-sidecar
python3.11 -m venv .venv
.venv/bin/pip install -i https://pypi.tuna.tsinghua.edu.cn/simple "crawl4ai==0.9.3" fastapi "uvicorn[standard]"
PLAYWRIGHT_DOWNLOAD_HOST=https://npmmirror.com/mirrors/playwright .venv/bin/crawl4ai-setup   # 装 chromium 等
```

## 启动（127.0.0.1:11235，与后端同机）

```bash
cd sg-scout-backend/deploy/crawl4ai-sidecar
.venv/bin/uvicorn main:app --host 127.0.0.1 --port 11235
# 健康检查
curl http://127.0.0.1:11235/health   # {"status":"ok"}
```

## 后端配置（config.toml，git-ignored）

```toml
[crawler.engine.crawl4ai]
base_url = "http://127.0.0.1:11235"
api_token = "local-sidecar"   # sidecar 不校验，非空即可（适配器契约）
```

- 未配置 api_token / 服务未起：任务执行响亮失败并提示（宪法 VI / FR-006）
- `GET /crawler/engines` 对 crawl4ai 做 /health 探活（1s 超时），available 反映可达性

## 已知行为差异（实测 2026-09-03）

- crawl4ai 自带反爬启发式会把「极小文本页」误判为被反爬拦截（success=false +
  error_message 含 minimal_text）——对真实内容页无影响（rjh 站实测通过），
  本地验收站点请保证页面正文达到正常规模
- sidecar 串行单浏览器（每请求一把锁）；节流由后端 BFS 驱动器保证（≥1s/页）

## 升级

- 钉版本 0.9.3；升级重装后重跑 quickstart.md 场景 2（rjh 站回归标尺）
- 若未来需要代理轮换/simulate_user 等 SDK 面能力：直接在 main.py 的
  CrawlerRunConfig 打开（sidecar 无 HTTP 参数白名单限制）

## systemd 伴生（iqa 部署机）

`ExecStart=<sidecar>/.venv/bin/uvicorn main:app --host 127.0.0.1 --port 11235`
（工作目录 deploy/crawl4ai-sidecar）；模式参考 sg-scout-backend.service。

## Docker 备选（不推荐，记录原因）

官方 `unclecode/crawl4ai:0.9.3` 镜像（deploy/docker 源码编译的完整 server，含
/crawl/job 与 Redis 依赖）——本机 registry 镜像源（docker.xuanyuan.me 等）对该
镜像 403、直连拉取中断，故弃用；若镜像源可用，按原方案 `docker run ... 11235`
并设 `CRAWL4AI_API_TOKEN`，适配器无需任何改动。
