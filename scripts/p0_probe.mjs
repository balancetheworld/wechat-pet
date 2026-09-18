// P0 基线验证探活脚本：验证 deepseek-chat 经 tokenflux.dev 的组合能力
// 用法：node scripts/p0_probe.mjs
// 从项目根目录 .env 读取 AI_API_KEY / AI_BASE_URL / AI_MODEL，不回显密钥。
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

function loadEnv() {
  const path = resolve(process.cwd(), '.env');
  const text = readFileSync(path, 'utf8');
  const env = {};
  for (const line of text.split('\n')) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) continue;
    const idx = trimmed.indexOf('=');
    if (idx < 0) continue;
    const key = trimmed.slice(0, idx).trim();
    const value = trimmed.slice(idx + 1).trim();
    env[key] = value;
  }
  return env;
}

const env = loadEnv();
const KEY = env.AI_API_KEY || '';
const BASE = (env.AI_BASE_URL || 'https://api.openai.com/v1').replace(/\/$/, '');
const MODEL = process.env.P0_MODEL || env.AI_MODEL || 'deepseek-chat';

if (!KEY) {
  console.error('AI_API_KEY missing in .env');
  process.exit(1);
}

function endpoint() {
  return `${BASE}/chat/completions`;
}

async function call(body, { stream = false } = {}) {
  const res = await fetch(endpoint(), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${KEY}`,
    },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    const text = await res.text().catch(() => '');
    return { ok: false, status: res.status, body: text.slice(0, 500) };
  }
  if (stream) {
    const text = await res.text();
    return { ok: true, status: res.status, streamText: text };
  }
  const json = await res.json();
  return { ok: true, status: res.status, json };
}

function report(name, ok, detail) {
  const tag = ok ? 'PASS' : 'FAIL';
  console.log(`[${tag}] ${name}`);
  if (detail) console.log(`      ${detail}`);
}

async function probe1Basic() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '回复两个字：收到' }],
    max_tokens: 64,
  });
  const content = r.json?.choices?.[0]?.message?.content;
  const ok = r.ok && !!content;
  report('基础文字调用', ok, r.ok ? `content=${JSON.stringify(content)}` : `status=${r.status} body=${r.body}`);
}

async function probe2Tools() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '查一下旺仔最近几次呕吐记录' }],
    tools: [
      { type: 'function', function: { name: 'query_health_records', description: '查询宠物健康记录', parameters: { type: 'object', properties: { pet: { type: 'string' }, symptom: { type: 'string' } }, required: ['pet'] } } },
    ],
    max_tokens: 200,
  });
  if (!r.ok) {
    report('工具调用(function calling)', false, `status=${r.status} body=${r.body}`);
    return;
  }
  const msg = r.json.choices?.[0]?.message;
  const toolCalls = msg?.tool_calls;
  report('工具调用(function calling)', !!toolCalls?.length, toolCalls?.length ? `tool_calls=${JSON.stringify(toolCalls.map(t => ({ id: t.id, name: t.function?.name, args: t.function?.arguments })))}` : `no tool_calls; content=${JSON.stringify(msg?.content)}`);
}

async function probe3Vision() {
  // 用本地图片转 base64 data URL，避免外部图片下载失败
  const imgPath = resolve(process.cwd(), 'data/uploads/067d49d58ce229174e2e6bb03f26867d.png');
  let dataUrl;
  try {
    const buf = readFileSync(imgPath);
    dataUrl = `data:image/png;base64,${buf.toString('base64')}`;
  } catch {
    report('图片理解(vision)', false, `本地图片读取失败: ${imgPath}`);
    return;
  }
  const r = await call({
    model: MODEL,
    messages: [
      { role: 'user', content: [
        { type: 'text', text: '请简短描述图片内容' },
        { type: 'image_url', image_url: { url: dataUrl } },
      ] },
    ],
    max_tokens: 64,
  });
  if (r.ok) {
    report('图片理解(vision)', true, `content=${JSON.stringify(r.json.choices?.[0]?.message?.content)}`);
  } else {
    report('图片理解(vision)', false, `status=${r.status} body=${r.body}`);
  }
}

async function probe4JsonMode() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '返回 JSON：{"a":1}' }],
    response_format: { type: 'json_object' },
    max_tokens: 64,
  });
  if (!r.ok) {
    report('结构化输出(json_object)', false, `status=${r.status} body=${r.body}`);
    return;
  }
  const content = r.json.choices?.[0]?.message?.content || '';
  let parsed = false;
  try { JSON.parse(content); parsed = true; } catch {}
  report('结构化输出(json_object)', parsed, `content=${JSON.stringify(content)}`);
}

async function probe5Stream() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '说一句你好' }],
    stream: true,
    max_tokens: 32,
  }, { stream: true });
  const hasData = r.ok && typeof r.streamText === 'string' && r.streamText.includes('data:');
  report('流式(stream=true)', hasData, r.ok ? `stream bytes=${r.streamText?.length}` : `status=${r.status} body=${r.body}`);
}

async function main() {
  console.log(`=== P0 探活 ${MODEL} @ ${BASE} ===`);
  await probe1Basic();
  await probe2Tools();
  await probe3Vision();
  await probe4JsonMode();
  await probe5Stream();
  console.log('=== done ===');
}

main().catch((err) => {
  console.error('probe error:', err);
  process.exit(1);
});
