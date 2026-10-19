# 全量使用记录设计

## 目标

在不依赖 Prompt Audit 或安全审计决策的前提下，记录所有 API 使用的元数据、用户内容、系统提示词、Developer Prompt、模型回复和工具消息，并按可靠的连续对话关系提供管理与分析能力。

## 边界

- `usage_logs` 继续作为计费、Token、模型、平台和耗时事实表。
- 新增独立的内容归档表，不把大段正文加入 `usage_logs`。
- 新模块只负责记录、检索和分析，不负责拦截、判定风险或替代安全审计。
- 二进制图片、文件、视频只记录元数据和 hash；正文内容不写入 PostgreSQL。

## 数据模型

`usage_archive_sessions` 表示连续使用会话，保存用户、API Key、分组、平台、模型、首末时间、turn 数量和连续性状态。

`usage_archive_turns` 表示一次 API 请求，关联 `usage_logs.request_id`，保存父 turn、序号、外部会话 ID、previous response ID、接口、状态、耗时和内容存储状态。

`usage_archive_messages` 保存拆分后的消息，角色必须区分 `system`、`developer`、`user`、`assistant`、`tool`、`unknown`；来源必须区分客户端请求、上游回复、上下文恢复和工具数据。正文可为空，空正文的消息保留长度、hash 和截断状态。

`usage_archive_prompt_snapshots` 对 system/developer prompt 按 hash 去重，turn 只引用快照 ID，避免同一提示词随每轮请求重复保存。

## 连续性

关联优先级为显式 conversation/thread/session ID、previous response ID、上游 response ID、provider history key，最后才允许短时间窗口推断。每条连接记录 `continuity_source` 和 `continuity_confidence`；无法可靠关联时创建独立 session。

## 存储策略

- 元数据长期保留，默认 90 天后按配置清理。
- 完整文本默认保留 7 天，之后只保留 hash、长度、角色、Token、费用和统计信息。
- 按 captured_at 月份分区或至少按时间批量清理。
- 仅建立 session/time/user/API Key/model 的查询索引；不默认建立正文全文索引。
- 记录每日归档字节数、正文行数和清理结果，在超过预算时停止正文写入但继续保存元数据。

## 管理能力

后台新增“全量使用记录”视图，支持按时间、用户、API Key、分组、平台、模型、接口、请求类型、状态、会话 ID、Request ID 查询；详情页分别展示用户输入、system/developer prompt、上下文、模型回复、工具链、请求元数据和计费数据。

分析视图提供调用量、Token、费用、首 Token 时间、总耗时、错误率、上下文长度、角色占比和连续会话 turn 数统计。

## 迁移策略

旧 `conversation_review_*` 数据只作为历史数据，不作为新模块的写入入口。新模块先从上线时刻开始记录；历史回填单独做 dry-run 统计，确认角色和连续性不会误判后再决定是否迁移。

