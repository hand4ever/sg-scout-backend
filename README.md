# SG Scout Backend

SG Scout 后端服务：基于 [Echo v5](https://github.com/labstack/echo) 的 Go Web 服务，采用分层架构搭建，内置统一响应格式、请求耗时统计、请求链路追踪等基础能力。参照 hand4ever/greetingfirst 工程骨架演化而来（非照抄，按 sg-scout 命名与起步需要裁剪）。

## 技术栈

| 项目 | 说明 |
|------|------|
| 语言 | Go 1.26 |
| Web 框架 | Echo v5.2.1 |
| ORM | GORM v1.31.2 (MySQL) |
| 配置文件 | TOML (`config.toml`) |
| 模块名 | `sg.scout` |
| 监听端口 | `:1324` |

> 端口避开本机常驻的 greeting 服务 (:1323)，如需还原改 `config.toml` 与 `config/config.go` 默认值即可。

## 目录结构

```
sg-scout-backend/
├── main.go            # 程序入口：初始化 Echo、注册中间件、启动服务
├── go.mod             # 模块定义与依赖
├── Makefile           # 本地开发 / 构建 / 测试脚本
├── config.toml        # 全局配置文件（TOML 格式）
├── config/            # 配置加载包（全局 Cfg 实例，缺失回落默认值）
├── router/            # 路由分组与注册
├── handler/           # 请求处理（控制器层）
├── entity/            # 请求参数 / 数据实体定义
├── model/             # 数据库映射层（GORM），全局 DB 实例
├── response/          # 统一 JSON 响应格式封装
└── middle/            # 自定义中间件
```

## 分层职责

| 目录 | 职责 |
|------|------|
| `router/` | 路由分组与注册，按业务模块划分 |
| `handler/` | 接收请求、参数绑定、调用响应封装 |
| `entity/` | 请求参数结构体（查询参数 / 路径参数等） |
| `model/` | 数据库映射模型（GORM），通过 `model.DB` 访问全局实例 |
| `response/` | 统一的成功 / 错误响应结构 |
| `middle/` | 自定义中间件（耗时统计等） |
| `config/` | 全局配置加载（TOML），通过 `config.Cfg` 访问 |

## 中间件

全局注册的中间件（按执行顺序）：

| 中间件 | 来源 | 作用 |
|--------|------|------|
| `RequestLogger` | Echo 内置 | 请求日志 |
| `Recover` | Echo 内置 | panic 恢复，避免进程崩溃 |
| `CORS` | Echo 内置 | 跨域资源共享，允许前端从不同源访问 API |
| `RequestID` | Echo 内置 | 生成请求追踪 ID（`X-Request-ID`） |
| `CostTime` | 自定义 | 记录请求耗时，并写入响应 `cost` 字段 |

## 统一响应格式

所有接口统一返回如下 JSON：

```json
{
  "code": 0,
  "message": "",
  "data": {},
  "trace_id": "请求 ID",
  "cost": "处理耗时，如 1.234ms",
  "extra": "可选扩展字段"
}
```

业务错误码定义（见 `response/message.go`）：

| 常量 | 值 | 说明 |
|------|------|------|
| `ErrCodeOk` | `0` | 成功 |
| `ErrCodeCustom` | `100001` | 通用业务错误 |
| `ErrCodeNetwork` | `100002` | 网络错误 |
| `ErrCodeDBDown` | `100003` | 数据库不可达 |

## API 列表

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/common/health` | 存活探针，返回 `{status, server_time}` |
| GET | `/common/version` | 应用版本信息（版本号、构建时间、Go 版本） |
| GET | `/common/setting` | 应用配置信息（名称、版本、端口、MySQL DSN） |
| GET | `/demo/search?tag=a&tag=b` | 演示：多值查询参数绑定 |
| GET | `/demo/echo/:str` | 演示：路径参数绑定 |

爬虫/设置接口见 feature 001/002 契约（`../sg-scout/specs/00*/contracts/api.md`）与 `api.http`。

### 校对模块 API（feature 004-text-proofreading，契约见 `../sg-scout/specs/004-text-proofreading/contracts/api.md`）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/proofreads` | 校对文档列表（?source= / ?page_id= 过滤） |
| POST | `/proofreads` | 创建校对文档（source_type=page\|text） |
| GET | `/proofreads/:id` | 文档详情（底稿全文+卡片+派生链+升级提示） |
| DELETE | `/proofreads/:id` | 删除文档（级联卡片+日志） |
| POST | `/proofreads/:id/upgrade` | 升级页面底稿至最新版本（卡片重置待确认） |
| POST | `/proofreads/:id/cards` | 新建校对卡片（重叠 400，服务端取原文） |
| PATCH | `/proofreads/:id/cards/:cid` | 编辑卡片字段（不改状态） |
| DELETE | `/proofreads/:id/cards/:cid` | 删除卡片 |
| POST | `/proofreads/:id/cards/:cid/state` | 状态改判（pending/accepted/rejected） |
| GET | `/proofreads/:id/logs` | 校对日志（只读，倒序） |
| GET | `/proofreads/:id/revision` | 修订稿预览（revised+marks） |
| GET | `/proofreads/:id/revision/export` | 导出纯净修订稿（.md） |
| GET | `/proofreads/:id/errata/export` | 导出勘误表（CSV，仅已接受） |
| POST | `/proofreads/:id/revision-doc` | 基于修订稿派生新校对文档 |

### 自动校对引擎 API（feature 005-auto-proofreading，契约见 `../sg-scout/specs/005-auto-proofreading/contracts/api.md`）

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/proofreads/engines/types` | 引擎类型注册表（lexicon/llm/httpapi + 配置字段） |
| GET | `/proofreads/engines` | 引擎实例列表 |
| POST | `/proofreads/engines` | 创建引擎实例（默认停用；启用即校验） |
| PATCH | `/proofreads/engines/:eid` | 更新引擎实例（含启用/停用） |
| DELETE | `/proofreads/engines/:eid` | 删除引擎实例（历史卡片保留来源快照） |
| POST | `/proofreads/:id/auto-check` | 开始自动校对（异步，全部已启用引擎） |
| GET | `/proofreads/:id/runs` | 自动校对运行记录（只读，倒序） |
| GET | `/proofreads/:id/runs/:rid` | 运行详情（引擎级状态/产出/耗时/失败原因） |
| GET | `/proofreads/:id?source=` | 卡片来源筛选（all/manual/ignored/engine/engine:{名}） |
| POST | `/proofreads/:id/cards/:cid/state` | 状态改判扩展：ignored + 撤回（pending）+ 接受冲突 409 |

## 更新日志

- 2026-09-04 feature 004：新增校对模块路由组 `/proofreads`（三表 schema/007-proofread.sql）
- 2026-09-04 feature 005：自动校对引擎层——引擎实例 CRUD（schema/008：ALTER proofread_card + proofread_engine/proofread_run 两表）、auto-check 异步运行、卡片来源/ignored 状态扩展、LLM provider 密钥走 config.toml `[proofread.providers.*]`（永不落 DB）

## 本地开发

```bash
make rundev      # 等价于 go run main.go
make build       # 编译当前平台到 bin/
make test        # 运行单元测试
make lint        # go vet 静态分析
```

启动后访问 `http://localhost:1324`。MySQL 未就绪时服务照常启动（连接失败仅告警），业务层如需使用 `model.DB` 请自行判空或按错误码处理。

## TODO / 待完善

- [ ] 补充业务接口与路由分组（按 `.specify/specs/` 中各 feature 推进）
- [ ] 启用自定义错误处理器，补充 404/500 等错误页
- [ ] 完善单元测试与 CI / 部署流程
