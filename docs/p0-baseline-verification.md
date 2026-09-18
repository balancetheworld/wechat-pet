# P0 基线验证记录

> 日期：2026-09-17。验证方式：`scripts/p0_probe.mjs` 用真实接口探活 + 现有代码核查。本文只记录结果，不承诺已通过的一期能力。

## 1. Provider 能力验证（真实接口）

### 1.1 关键发现：当前模型已失效

`.env` 当前配置 `AI_MODEL=deepseek-chat` 经 `https://tokenflux.dev/v1` 调用返回 **403**：

```
The current group does not support the requested model "deepseek-chat".
Available models: deepseek-flash
```

**结论**：必须切换到 `deepseek-flash`（tokenflux 当前唯一可用模型）。

### 1.2 deepseek-flash 能力矩阵（真实探活）

| 能力 | 结果 | 证据 |
|---|---|---|
| 基础文字 | ✅ | `content="收到"` |
| 工具调用 function calling | ✅ | 正确返回 `tool_calls` 并解析参数 `{"pet":"旺仔","symptom":"呕吐"}` |
| 图片理解 vision | ✅ | 正确描述图片「黄色卡通角色戴黑色小熊耳帽、手捧白色盒子」 |
| 结构化输出 json_object | ✅ | 正确返回 `{"a":1}` |
| 流式 stream | ✅ | SSE 正常返回（10930 bytes） |

### 1.3 关键约束：推理模型

`deepseek-flash` 是推理模型，输出预算先被 `reasoning_tokens` 消耗：

- `max_tokens=64` 时 vision 请求返回 `content=""`、`finish_reason=length`（reasoning 耗尽预算）。
- `max_tokens=4000` 时 vision 正常输出，`completion_tokens=157`、`reasoning_tokens=127`。

**对 v2 的影响**：P3 Provider 适配的 `max_tokens` 必须给足预算（建议 ≥ 2000），否则结构化输出/图片会被截断为空。

### 1.4 图片输入方式

`base64 data URL`（`data:image/jpeg;base64,...`）可行，vision 正常识别；外部 URL 需可下载。

## 2. 现有代码能力核查

| 能力 | 现状 | 缺口 |
|---|---|---|
| 事务/幂等/版本 | `repository.go` 用 BeginTx + 幂等键表 + row_version 条件更新 | 需补 `input_revision`、`execution_epoch`、来源版本 |
| 删除 | `storage.Storage` 有 `Delete`；业务表删除待查 | 需补删除标记、保留时间（聊天 30 天 / 审计 90 天） |
| 权限 | `middleware` 有 RequireAuth / RequireFamily / Owner | 需补来源版本失效、最小回执 |
| 保留/到期清理 | 无到期清理机制 | T9 实现 |

## 3. 待补充（未验证）

- tokenflux.dev 隐私条款、数据训练/留存/删除、媒体访问方式 —— 需查证后记录。
- 图片计价单位、用量缺失/迟到、取消行为、请求硬限制 —— P3 前补测。
- 兽医资料与版本化风险规则的固定样例 —— T4/T5 固定。

## 4. 落地动作

1. `.env` 的 `AI_MODEL` 由 `deepseek-chat` 改为 `deepseek-flash`（已处理）。
2. P3 设计 Provider 时考虑 `reasoning_tokens` 对 `max_tokens` 预算的影响。
3. 探活脚本保留为 `scripts/p0_probe.mjs`，可复用于后续接入验证。
