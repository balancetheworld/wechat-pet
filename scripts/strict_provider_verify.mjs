// record_array_v1 strict 结构化输出验证脚本：验证 Provider 是否按
// object 根 + records 数组（anyOf 八种记录）强制约束模型输出。
// 用法：node scripts/strict_provider_verify.mjs [每场景调用次数，默认 2] [--stream]
// 默认走非流式；加 --stream 走流式，用于核对生产决策循环所用的流式通道是否同样受 strict 约束。
// 从项目根目录 .env 读取 AI_API_KEY / AI_BASE_URL / AI_MODEL，不回显密钥。
// 依赖 go 工具链：通过 scripts/dump_record_schema.go 读取服务端固定 Schema，避免脚本内重复维护。
import { execFileSync } from 'node:child_process';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

const ROOT = process.cwd();
const args = process.argv.slice(2);
const STREAM = args.includes('--stream');
const ROUNDS = Number(args.find(value => !value.startsWith('--')) || 2);

function loadEnv() {
  const env = {};
  for (const line of readFileSync(resolve(ROOT, '.env'), 'utf8').split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const index = trimmed.indexOf('=');
    if (index < 0) continue;
    env[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1).trim();
  }
  return env;
}

const env = loadEnv();
const KEY = env.AI_API_KEY || '';
const BASE = (env.AI_BASE_URL || 'https://api.openai.com/v1').replace(/\/$/, '');
const MODEL = env.AI_MODEL || '';
if (!KEY || !MODEL) {
  console.error('AI_API_KEY / AI_MODEL missing in .env');
  process.exit(1);
}

const schema = JSON.parse(execFileSync('go', ['run', './scripts/dump_record_schema.go'], { cwd: ROOT, encoding: 'utf8' }));

// 受控指令由服务端提供（ControlInstructions），此处只复述与协议外壳相关的部分。
const instructions = `你是「宠物问问」助手，为家庭宠物提供健康观察、记录查询与日常照护建议。
回答必须以记录数组形式给出：header 开场、coverage + end 收尾。
call 记录的 arguments 是 JSON 字符串（内部键值对需转义），不要直接写 JSON 对象。
所有字段都必须出现：没有内容的数组写 []、没有内容的字符串写 null。`;

const scenarios = [
  { name: 'greeting', input: '你好', needTools: false },
  {
    name: 'tool_query',
    input: '旺仔上次打疫苗是什么时候',
    needTools: true,
    toolCatalog: [{ name: 'search_health_records', version: 'v1', description: '查询宠物健康记录', parameters: { type: 'object', properties: { pet_name: { type: 'string' }, category: { type: 'string' } }, required: ['pet_name'] } }],
  },
];

function buildBody(scenario) {
  let text = instructions;
  if (scenario.needTools) {
    text += `\n\n【可用工具】\n仅在 record_array_v1 的 call 记录中提出工具调用，不要使用原生工具调用。\n${JSON.stringify(scenario.toolCatalog)}`;
  }
  return {
    model: MODEL,
    store: false,
    stream: STREAM,
    text: { format: { type: 'json_schema', name: 'pet_ask', strict: true, schema } },
    instructions: text,
    input: [{ type: 'message', role: 'user', content: [{ type: 'input_text', text: scenario.input }] }],
    max_output_tokens: 4096,
  };
}

function outputText(payload) {
  return (payload.output || [])
    .flatMap(item => item.content || [])
    .filter(content => content.type === 'output_text')
    .map(content => content.text || '')
    .join('');
}

// collectStream 读取 Responses API 的 SSE 流，拼接正文增量并记录未完成或失败原因。
async function collectStream(response) {
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let text = '';
  let incompleteReason = '';
  let failedReason = '';
  for (;;) {
    const { value, done } = await reader.read();
    if (done) break;
    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';
    for (const line of lines) {
      if (!line.startsWith('data:')) continue;
      const data = line.slice(5).trim();
      if (!data || data === '[DONE]') continue;
      let event;
      try {
        event = JSON.parse(data);
      } catch {
        continue;
      }
      if (event.type === 'response.output_text.delta') text += event.delta || '';
      if (event.type === 'response.incomplete') incompleteReason = event.response?.incomplete_details?.reason || 'unknown';
      if (event.type === 'response.failed') failedReason = event.response?.error?.message || 'failed';
    }
  }
  return { text, incompleteReason, failedReason };
}

function inspect(scenario, text) {
  const result = { valid_json: false, records: null, arguments_ok: null, error: null };
  try {
    const parsed = JSON.parse(text);
    result.valid_json = Array.isArray(parsed.records);
    result.records = (parsed.records || []).map(record => record.type);
    const calls = (parsed.records || []).filter(record => record.type === 'call');
    result.arguments_ok = calls.every((call) => {
      if (typeof call.arguments === 'string') {
        try {
          return typeof JSON.parse(call.arguments) === 'object';
        } catch {
          return false;
        }
      }
      return call.arguments !== null && typeof call.arguments === 'object';
    });
  } catch (error) {
    result.error = String(error).slice(0, 120);
    return result;
  }
  if (scenario.needTools && result.records && !result.records.includes('call') && !result.records.includes('question')) {
    result.error = 'expected call_tools or request_input';
  }
  return result;
}

let failed = 0;
for (const scenario of scenarios) {
  for (let round = 1; round <= ROUNDS; round += 1) {
    const started = Date.now();
    const response = await fetch(`${BASE}/responses`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${KEY}` },
      body: JSON.stringify(buildBody(scenario)),
    });
    const elapsed = Date.now() - started;
    const label = `${scenario.name}#${round}`;
    if (!response.ok) {
      failed += 1;
      const body = await response.text();
      console.log(`[FAIL] ${label} status=${response.status} ${body.slice(0, 200)}`);
      continue;
    }
    let result;
    let note = '';
    if (STREAM) {
      const streamed = await collectStream(response);
      result = inspect(scenario, streamed.text);
      note = `${streamed.failedReason ? ` failed=${streamed.failedReason}` : ''}${streamed.incompleteReason ? ` incomplete=${streamed.incompleteReason}` : ''}`;
    } else {
      const payload = await response.json();
      result = inspect(scenario, outputText(payload));
    }
    const ok = result.valid_json && result.arguments_ok !== false && !result.error;
    if (!ok) failed += 1;
    console.log(`[${ok ? 'PASS' : 'FAIL'}] ${label} stream=${STREAM} elapsed=${elapsed}ms records=${JSON.stringify(result.records)} arguments_ok=${result.arguments_ok}${result.error ? ` error=${result.error}` : ''}${note}`);
  }
}

console.log(failed === 0 ? `strict 验证通过（stream=${STREAM}）：全部调用都符合 record_array_v1` : `strict 验证失败（stream=${STREAM}）：${failed} 次调用不符合契约`);
process.exit(failed === 0 ? 0 : 1);
