# 宠物问问接口

以下接口均需要 JWT，并要求当前用户属于一个 active 家庭。所有 `pet_id`、`session_id` 和 `run_id` 都会在服务端按当前家庭重新校验。

## 创建问问会话

`POST /api/v1/pets/:pet_id/ask/sessions`

或由问题中的宠物名称自动定位单只或多只宠物：

`POST /api/v1/ask/sessions`

请求必须携带：

```http
Idempotency-Key: ask-client-generated-key
```

幂等键不能为空且不能超过 128 个字符。同一用户、同一操作使用相同幂等键和相同请求时返回首次结果；相同幂等键用于不同请求时返回 409。

请求体：

```json
{
  "input": "最近两天食欲不太好"
}
```

当前版本先在事务中创建排队中的 Run，再投递到进程内 Worker 异步执行。接口响应仍返回事务提交时的 `queued` 快照，后续状态通过 Snapshot 或事件流获取。

响应中的关键字段：

```json
{
  "code": 0,
  "data": {
    "session": {
      "id": "session-id",
      "pet_id": "pet-id",
      "status": "active",
      "risk_level": "unknown",
      "turn_count": 1
    },
    "run": {
      "id": "run-id",
      "turn_id": "turn-id",
      "run_index": 0,
      "row_version": 1,
      "clarification_count": 0,
      "attempt_count": 0,
      "status": "queued",
      "risk_level": "unknown",
      "next_attempt_at": null
    },
    "events": [
      {
        "sequence": 1,
        "type": "run.queued",
        "data": {}
      }
    ]
  }
}
```

## 执行问问 Run

`POST /api/v1/ask/sessions/:session_id/runs/:run_id/process`

该接口保留为兼容和人工重试入口。正常创建和 Reply 流程不需要调用；接口读取当前 Run，若仍为 `queued` 则重新投递 Worker，不会在 HTTP 请求内直接执行。当前使用确定性 Executor，成功后会进入 `waiting_input` 并写入追问事件。

可能的 Run 状态：

```text
waiting_input
completed
escalated
failed
```

Worker 每次执行前会领取数据库租约。基础设施错误会写入 `run.retry_scheduled` 并在 `next_attempt_at` 后重试；进程中断留下的过期 `running` Run 会写入 `run.recovered` 后重新执行。超过最大尝试次数会写入 `run.failed`，`error_code` 为 `worker_attempts_exhausted`。

## 回复问问追问

`POST /api/v1/ask/sessions/:session_id/runs/:run_id/reply`

只有状态为 `waiting_input` 的 Run 可以接收回答。请求必须携带新的 `Idempotency-Key`，回答成功后原 Run 恢复为 `queued`，不会创建新的 Turn 或 Run。

请求体：

```json
{
  "input": "从今天早上开始，已经吐了两次",
  "expected_version": 3
}
```

`expected_version` 必须等于当前 Run 的 `row_version`。事务成功后：

```text
waiting_input(row_version=N)
→ queued(row_version=N+1, clarification_count+1)
```

同一事务还会写入用户回答、追加新的 `run.queued` 事件和幂等结果，提交后将新版本 Run 投递到 Worker。回答内容不能为空，长度不能超过 4000 个字符。最多允许三次追问补充；达到上限后若 Agent 仍要求追问，Run 会收敛为 `failed`。

## 查询问问会话

`GET /api/v1/ask/sessions/:session_id`

只返回当前家庭中的会话。不存在或不属于当前家庭时返回对应错误。

## 查询完整会话 Snapshot

`GET /api/v1/ask/sessions/:session_id/snapshot`

返回 Session、绑定宠物、全部 Turn、每个 Turn 的全部 Run、选中 Run 快捷字段、Run 消息、Run 事件和 `event_cursors`。`runs` 中每项包含 `run`、`messages` 和 `events`；快捷字段对应 `turn.selected_run_id`。`messages` 包含 `role`、`content` 和 `created_at`，用于恢复同一 Run 内的原始问题、Agent 追问和用户补充回答。前端页面恢复时以该接口为权威状态，再从当前 Run 的最新事件游标继续消费增量。

## 查询问问事件

`GET /api/v1/ask/sessions/:session_id/runs/:run_id/events?after=1`

`after` 为可选事件序号游标，默认从 0 开始。服务端会同时验证 Session 和 Run 的归属关系。

响应事件字段：

```json
{
  "run_id": "run-id",
  "sequence": 2,
  "type": "run.started",
  "data": {},
  "created_at": "2026-09-08T00:00:00Z"
}
```

## 流式读取问问事件

`GET /api/v1/ask/sessions/:session_id/runs/:run_id/events/stream?after=1`

响应类型为 `application/x-ndjson`，每一行是一个完整事件对象，不使用公共 JSON 响应包装。连接结束后客户端使用最新 `sequence` 作为 `after` 重连。
