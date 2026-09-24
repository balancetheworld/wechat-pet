// 推理流下发探活脚本：验证网关是否接受 reasoning.summary，以及流式响应中是否出现
// 思考增量事件（response.reasoning_summary_text.delta）。
// 用法：node scripts/reasoning_stream_probe.mjs
// 从项目根目录 .env 读取 AI_API_KEY / AI_BASE_URL / AI_MODEL，不回显密钥。
import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

const ROOT = process.cwd()

function loadEnv() {
  const env = {}
  for (const line of readFileSync(resolve(ROOT, '.env'), 'utf8').split('\n')) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const index = trimmed.indexOf('=')
    if (index < 0) continue
    env[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1).trim()
  }
  return env
}

const env = loadEnv()
const KEY = env.AI_API_KEY || ''
const BASE = (env.AI_BASE_URL || 'https://api.openai.com/v1').replace(/\/$/, '')
const MODEL = env.AI_MODEL || ''
if (!KEY || !MODEL) {
  console.error('AI_API_KEY / AI_MODEL missing in .env')
  process.exit(1)
}

async function probe(name, reasoning) {
  const body = {
    model: MODEL,
    store: false,
    stream: true,
    input: [{ type: 'message', role: 'user', content: [{ type: 'input_text', text: '旺仔是一只三岁的布偶猫，今天食欲有点差。用一句话给建议。' }] }],
    max_output_tokens: 2048,
  }
  if (reasoning) {
    Object.assign(body, reasoning)
  }
  const started = Date.now()
  const response = await fetch(`${BASE}/responses`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${KEY}` },
    body: JSON.stringify(body),
  })
  if (!response.ok) {
    const text = await response.text()
    console.log(`[${name}] HTTP ${response.status} ${text.slice(0, 500)}`)
    return
  }
  const types = []
  let reasoningText = ''
  let outputText = ''
  let reasoningEvents = 0
  let firstReasoningAt = null
  let firstOutputAt = null
  const reader = response.body.getReader()
  const decoder = new TextDecoder()
  let buffer = ''
  for (;;) {
    const { value, done } = await reader.read()
    if (done) break
    buffer += decoder.decode(value, { stream: true })
    const lines = buffer.split('\n')
    buffer = lines.pop() ?? ''
    for (const line of lines) {
      if (!line.startsWith('data:')) continue
      const payload = line.slice(5).trim()
      if (!payload || payload === '[DONE]') continue
      let event
      try {
        event = JSON.parse(payload)
      }
      catch {
        continue
      }
      types.push(event.type)
      if (event.type === 'response.reasoning_text.delta' || event.type === 'response.reasoning_summary_text.delta') {
        reasoningEvents++
        if (firstReasoningAt === null) firstReasoningAt = Date.now() - started
        reasoningText += event.delta ?? ''
      }
      if (event.type === 'response.output_text.delta') {
        if (firstOutputAt === null) firstOutputAt = Date.now() - started
        outputText += event.delta ?? ''
      }
    }
  }
  console.log(`[${name}] total=${Date.now() - started}ms firstReasoning=${firstReasoningAt ?? '-'}ms firstOutput=${firstOutputAt ?? '-'}ms`)
  console.log(`[${name}] events=${[...new Set(types)].join(' | ')}`)
  console.log(`[${name}] reasoningEvents=${reasoningEvents}`)
  console.log(`[${name}] reasoning=${JSON.stringify(reasoningText.slice(0, 200))}`)
  console.log(`[${name}] output=${JSON.stringify(outputText.slice(0, 200))}`)
}

await probe('with-summary', { summary: 'auto' })
await probe('without-reasoning', null)

const schema = JSON.parse(execFileSync('go', ['run', './scripts/dump_record_schema.go'], { cwd: ROOT, encoding: 'utf8' }))
await probe('strict-json-schema', {
  text: { format: { type: 'json_schema', name: 'pet_ask', strict: true, schema } },
  instructions: '你是宠物健康助手。回答必须以记录数组形式给出：header 开场、coverage + end 收尾。所有字段都必须出现：没有内容的数组写 []、没有内容的字符串写 null。',
})
