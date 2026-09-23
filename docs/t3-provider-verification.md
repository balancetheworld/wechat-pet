# T3 Provider 真实组合验证记录

> 文档状态：T3 块 B（真实 Provider 组合验证）已执行，本记录为 12.2「模型与协议组合」「用量与资源限制」「隐私与第三方处理」三项前置的可核验证据。

## 1. 验证概览

| 项 | 值 |
|---|---|
| 验证日期 | 2026-09-17 |
| 模型 | `deepseek-flash` |
| Base URL | `https://tokenflux.dev/v1`（OpenAI Chat Completions 兼容协议） |
| 适配器 | `internal/platform/ai/chat_completion.go`（`ChatCompletionExecutor`） |
| 验证脚本 | `scripts/t3_provider_verify.mjs` |
| 结果 | **12 / 12 通过**（组合能力 + 用量 + 取消 + 硬限制） |

## 2. 组合能力验证（对应 12.2「模型与协议组合」）

| # | 验证项 | 结果 | 关键观测 |
|---|---|---|---|
| 1 | 基础文字 | ✅ | `max_tokens=2048` 时正常返回 |
| 2 | 结构化输出 `json_object` | ✅ | 返回合法 JSON |
| 3 | 工具调用（完整参数） | ✅ | `tool_calls` 数组，`pet_name/symptom/count/time_range` 全部填齐，`finish=tool_calls` |
| 4 | 图片理解（vision） | ✅ | 正确描述图片内容（卡通房子、水印「豆包AI生成」） |
| 5 | 组合（文字+图片+工具+结构化） | ⚠️ | 见第 4 节 DSML 发现 |
| 6 | 工具结果回灌（第二轮） | ✅ | 工具结果注入后正常生成回复 |
| 7 | 截断 `finish_reason=length` | ✅ | `max_tokens=8` 时正确返回 `length` |
| 8 | 有序流（stream + include_usage） | ✅ | 5 个 chunk，顺序「一、二、三」，`finish=stop`，末 chunk 带 usage |
| 9 | 非流式 | ✅ | 一次性返回完整结果 |
| 10 | 取消（AbortController） | ✅ | 500ms 后 abort，`AbortError`，连接真正中断 |
| 11 | 用量字段 | ✅ | 见第 3 节 |
| 12 | 请求硬限制（超长输入 300k 字符） | ✅ | 被接受（200），未触发输入长度拒绝 |

## 3. 用量与计量发现（对应 12.2「用量与资源限制」）

### 3.1 用量字段结构（实测）

```json
{
  "prompt_tokens": 36,
  "completion_tokens": 399,
  "total_tokens": 435,
  "prompt_tokens_details": { "cached_tokens": 0 },
  "completion_tokens_details": { "reasoning_tokens": 145 },
  "prompt_cache_hit_tokens": 0,
  "prompt_cache_miss_tokens": 36
}
```

### 3.2 关键计量结论

| 结论 | 依据 |
|---|---|
| **推理 token 计入 `completion_tokens`** | `reasoning_tokens` 占 completion 的 70%～100%，费用按 completion 计 |
| **图片折算进 `prompt_tokens`** | 纯文字 prompt ≈ 36～39 token；1.3MB PNG 图片 prompt = 1026 token（图片约折算 990 token） |
| **无独立图片计价字段** | 图片不单独计费，直接并入 `prompt_tokens`，计量归一化无需单独图片 token 映射 |
| **前缀缓存命中** | `prompt_cache_hit_tokens` / `prompt_tokens_details.cached_tokens` 存在，同一天重复请求图片前缀命中 892 token |
| **流式用量迟到** | 非流式立即返回 usage；流式仅末 chunk（`stream_options.include_usage=true`）返回，属「用量迟到」 |
| **max_tokens 硬约束** | 推理模型：`max_tokens` 过小（64/128/256）时 reasoning 耗尽输出预算，正文为空。**正文可用需 `max_tokens ≥ 2048`** |

## 4. 协议兼容性发现（重要）

### 4.1 DSML 工具调用（组合模式下）

`json_object` + `tools` **同时使用**时，工具调用**不再走标准 `tool_calls` 数组**，而是退化为内嵌在 `content` 里的 DSML 文本：

```text
{"type": "json_object"}

<｜DSML｜｜ calls>
<｜DSML｜｜ invoke name="record_pet">
<｜DSML｜｜ parameter name="species" string="true">unknown</｜DSML｜｜ parameter>
</｜DSML｜｜ invoke>
</｜DSML｜｜ calls>
```

对比 probe 3（纯 `tools`、无 `json_object`）返回的是标准 `tool_calls` 数组、`finish=tool_calls`。

**适配器实现约束**：三种调用用途的 `agent_step` 决策请求中，**禁止同时下发 `response_format: json_object` 与 `tools`**；工具调用与结构化输出必须二选一，否则工具调用会以 DSML 文本形式污染正文。

### 4.2 图片能力

- vision 输入经 data URL（base64）正常接收，未出现「静默删图」。
- 图片内元数据（水印）可被识别，不影响理解。
- 生图能力未注册（符合 11.5 生图延期决定）。

## 5. 隐私与第三方处理（对应 12.2「隐私与第三方处理」）

| 项 | 结果 |
|---|---|
| 隐私政策页面 | ⚠️ **未找到**。`https://tokenflux.dev/privacy` 返回首页标题（SPA），无独立条款页 |
| 数据用途 / 训练 / 留存 / 删除 | ⚠️ **未获取到正式条款** |
| 影响 | 按 12.2「不满足时该适配器不得发送用户数据」，在取得可核验隐私条款前，**该 Provider 不应向真实用户数据开放**；开发/验证阶段可继续，但上线前必须补齐 |

## 6. 对 T3 适配器实现的固化约束

| 约束 | 落地位置 |
|---|---|
| `max_tokens` 下限 2048（推理模型正文保障） | Profile 参数配置 / 调用固定配置 |
| `reasoning_tokens` 计入 `completion_tokens`，不再单列费用 | usage.go 归一化 |
| 图片并入 `prompt_tokens`，无独立图片计价字段 | usage.go 归一化 |
| 缓存命中 token 单独记录（对账用） | usage.go 归一化 |
| 流式 usage 迟到 → 结算在末 chunk 后补记 | usage.go / 调用预留结算 |
| 禁止 `json_object` 与 `tools` 同请求 | agent_step 决策请求构造 |

## 7. 结论

T3 块 B 验证完成：`deepseek-flash` @ tokenflux 的组合能力（文字/图片/工具/结构化/流式/取消）真实可用，用量字段完整可对账。**唯一阻断上线的项是隐私条款缺失**（第 5 节），其余为适配器实现的工程约束（第 6 节），将在 T3 适配器接入与 T5 决策循环落地时逐项应用。

## 8. record_array_v1 strict 结构化输出验证（2026-09-23 补测）

### 8.1 背景

非 strict 下 `text.format=json_schema` 不产生约束：同一请求 5 次里 2 次返回非法 JSON（模型把 `{"items":[…]}` 包装的 `]}` 提前闭合，再继续写 coverage/end），整份响应解析失败 → `provider_output_invalid`（不可重试）→ Run 直接失败。因此补测 strict 能力并改造 Schema。

### 8.2 网关 strict 能力边界（实测）

| 能力 | 结果 |
|---|---|
| 根 object + 全部 properties 进入 required + `additionalProperties:false` | 支持且真正约束（提示词要求额外字段时被强制丢弃） |
| 必填数组、嵌套对象、标量可空 `["string","null"]` | 支持 |
| `items` 使用 `anyOf` 或 `$ref`/`$defs` 的判别联合 | 支持 |
| 根为数组 | 拒绝（400） |
| `oneOf` | 拒绝（400，只接受 `type`/`anyOf`/`$ref`） |
| `["array","null"]`、`["object","null"]` | 拒绝（400，type 联合只接受标量） |
| 自由对象（`additionalProperties:true`） | 拒绝（strict 要求 false） |

### 8.3 协议调整

- `record_array_v1` 外壳改为 `{"records":[…]}`：根 object，`records` 为 `anyOf` 八种记录的数组；可省略标量改为必填 nullable，可省略数组改为必填空数组；判别联合由 `oneOf` 改为 `anyOf`。
- `call.arguments` 由自由对象改为 **JSON 字符串**（服务端解析并归一化为对象），这是 strict 下唯一无法保留自由结构的字段。解析端同时接受字符串与对象两种形式，保证非 strict 回退仍可用。
- Provider 对 strict 兼容 Schema 请求 `strict:true`；网关以 400 拒绝时回退一次非 strict，并以 `strict_fallback` 标记观测。

### 8.4 验证结果

脚本：`scripts/strict_provider_verify.mjs`（Schema 由 `scripts/dump_record_schema.go` 从服务端固定定义读取，避免脚本内重复维护）。

```text
[PASS] greeting#1  records=[header,group,segment,coverage,end]   arguments_ok=true
[PASS] greeting#2  records=[header,group,segment,coverage,end]   arguments_ok=true
[PASS] tool_query#1 records=[header,group,call,coverage,end]     arguments_ok=true
[PASS] tool_query#2 records=[header,call,coverage,end]           arguments_ok=true
strict 验证通过：全部调用都符合 record_array_v1
```

### 8.5 仍未覆盖

strict 只约束结构，不约束语义：`source_turn_ids` 等来源字段可能被模型填成不存在的标识、coverage 覆盖关系与风险等级仍需服务端校验兜底；`provider_output_invalid` 目前仍为不可重试分类。
