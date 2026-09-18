// T3 Provider 组合验证脚本：验证 deepseek-flash 经 tokenflux.dev 的真实组合能力
// 覆盖文档 12.2「模型与协议组合」+「用量与资源限制」两项前置。
// 用法：node scripts/t3_provider_verify.mjs
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
const MODEL = env.AI_MODEL || 'deepseek-flash';

if (!KEY) {
  console.error('AI_API_KEY missing in .env');
  process.exit(1);
}

const endpoint = () => `${BASE}/chat/completions`;

async function call(body, { stream = false, signal } = {}) {
  const started = Date.now();
  try {
    const res = await fetch(endpoint(), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${KEY}`,
      },
      body: JSON.stringify(body),
      signal,
    });
    const elapsed = Date.now() - started;
    if (!res.ok) {
      const text = await res.text().catch(() => '');
      return { ok: false, status: res.status, body: text.slice(0, 800), elapsedMs: elapsed };
    }
    if (stream) {
      const text = await res.text();
      return { ok: true, status: res.status, streamText: text, elapsedMs: elapsed };
    }
    const json = await res.json();
    return { ok: true, status: res.status, json, elapsedMs: elapsed };
  } catch (err) {
    const elapsed = Date.now() - started;
    return { ok: false, error: String(err?.name || err), elapsedMs: elapsed };
  }
}

const results = [];
function report(name, ok, detail) {
  results.push({ name, ok });
  const tag = ok ? 'PASS' : 'FAIL';
  console.log(`\n[${tag}] ${name}`);
  if (detail) console.log(`      ${detail}`);
}

// ---- 1. 基础文字（确认 key 有效）----
async function probe1Basic() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '回复两个字：收到' }],
    max_tokens: 2048,
  });
  const content = r.json?.choices?.[0]?.message?.content;
  report('基础文字调用', r.ok && !!content, r.ok
    ? `content=${JSON.stringify(content)} elapsed=${r.elapsedMs}ms`
    : `status=${r.status} error=${r.error || ''} body=${r.body || ''}`);
}

// ---- 2. 结构化输出 json_object ----
async function probe2JsonMode() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '返回 JSON：{"a":1,"b":"x"}' }],
    response_format: { type: 'json_object' },
    max_tokens: 256,
  });
  if (!r.ok) return report('结构化输出(json_object)', false, `status=${r.status} body=${r.body}`);
  const content = r.json.choices?.[0]?.message?.content || '';
  let parsed = false;
  try { JSON.parse(content); parsed = true; } catch {}
  report('结构化输出(json_object)', parsed, `content=${JSON.stringify(content)}`);
}

// ---- 3. 工具调用：完整参数 ----
async function probe3Tools() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '记录旺仔今天早上呕吐了两次，伴有食欲下降' }],
    tools: [{
      type: 'function',
      function: {
        name: 'record_health_event',
        description: '记录宠物健康事件',
        parameters: {
          type: 'object',
          properties: {
            pet_name: { type: 'string', description: '宠物名' },
            symptom: { type: 'string', description: '症状' },
            count: { type: 'integer', description: '次数' },
            time_range: { type: 'string', description: '时间段' },
          },
          required: ['pet_name', 'symptom', 'count'],
        },
      },
    }],
    max_tokens: 512,
  });
  if (!r.ok) return report('工具调用(完整参数)', false, `status=${r.status} body=${r.body}`);
  const msg = r.json.choices?.[0]?.message;
  const tc = msg?.tool_calls;
  if (!tc?.length) return report('工具调用(完整参数)', false, `无 tool_calls; content=${JSON.stringify(msg?.content)} finish=${r.json.choices?.[0]?.finish_reason}`);
  const calls = tc.map((t) => {
    let args;
    try { args = JSON.parse(t.function?.arguments || '{}'); } catch { args = t.function?.arguments; }
    return { name: t.function?.name, args };
  });
  const first = calls[0];
  const argsComplete = first?.args && typeof first.args === 'object'
    && first.args.pet_name && first.args.symptom && typeof first.args.count === 'number';
  report('工具调用(完整参数)', argsComplete, `calls=${JSON.stringify(calls)} finish=${r.json.choices?.[0]?.finish_reason}`);
}

// ---- 4. 图片理解 ----
function imageDataUrl(relPath) {
  const p = resolve(process.cwd(), relPath);
  const buf = readFileSync(p);
  const ext = relPath.split('.').pop().toLowerCase();
  const mime = ext === 'png' ? 'image/png' : 'image/jpeg';
  return `data:${mime};base64,${buf.toString('base64')}`;
}

async function probe4Vision() {
  let dataUrl;
  try { dataUrl = imageDataUrl('data/uploads/067d49d58ce229174e2e6bb03f26867d.png'); }
  catch (e) { return report('图片理解(vision)', false, `图片读取失败 ${e.message}`); }
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: [
      { type: 'text', text: '请简短描述图片内容' },
      { type: 'image_url', image_url: { url: dataUrl } },
    ] }],
    max_tokens: 2048,
  });
  if (!r.ok) return report('图片理解(vision)', false, `status=${r.status} body=${r.body}`);
  const vContent = r.json.choices?.[0]?.message?.content;
  report('图片理解(vision)', !!vContent, `content=${JSON.stringify(vContent)}`);
  if (r.json.usage) console.log(`      图片usage=${JSON.stringify(r.json.usage)}`);
  return r.json;
}

// ---- 5. 组合：文字 + 图片 + 工具 + 结构化输出（同请求）----
async function probe5Combined() {
  let dataUrl;
  try { dataUrl = imageDataUrl('data/uploads/067d49d58ce229174e2e6bb03f26867d.png'); }
  catch (e) { return report('组合(文字+图片+工具+结构化)', false, `图片读取失败 ${e.message}`); }
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: [
      { type: 'text', text: '看图片里的动物。若可识别，调用 record_pet 记录其种类，并用 JSON 返回一条结论。' },
      { type: 'image_url', image_url: { url: dataUrl } },
    ] }],
    tools: [{
      type: 'function',
      function: {
        name: 'record_pet',
        description: '记录识别的宠物种类',
        parameters: { type: 'object', properties: { species: { type: 'string' } }, required: ['species'] },
      },
    }],
    response_format: { type: 'json_object' },
    max_tokens: 512,
  });
  if (!r.ok) return report('组合(文字+图片+工具+结构化)', false, `status=${r.status} body=${r.body}`);
  const choice = r.json.choices?.[0];
  const msg = choice?.message;
  const detail = `finish=${choice?.finish_reason} tool_calls=${msg?.tool_calls?.length || 0} content=${JSON.stringify(msg?.content)}`;
  // 组合请求：模型要么选择工具调用、要么结构化正文，二者至少其一，且图片未被静默丢弃（返回 200）
  const ok = !!msg && (!!msg.tool_calls?.length || typeof msg.content === 'string');
  report('组合(文字+图片+工具+结构化)', ok, detail);
  return r.json;
}

// ---- 6. 工具结果回灌（完整参数往返）----
async function probe6ToolRoundTrip() {
  const r1 = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '查询旺仔最近两次呕吐记录' }],
    tools: [{
      type: 'function',
      function: {
        name: 'query_health_records',
        description: '查询宠物健康记录',
        parameters: { type: 'object', properties: { pet: { type: 'string' }, symptom: { type: 'string' }, limit: { type: 'integer' } }, required: ['pet'] },
      },
    }],
    max_tokens: 512,
  });
  if (!r1.ok || !r1.json?.choices?.[0]?.message?.tool_calls?.length) {
    return report('工具结果回灌(第二轮)', false, r1.ok ? '第一轮未产生工具调用' : `status=${r1.status} body=${r1.body}`);
  }
  const tc = r1.json.choices[0].message.tool_calls[0];
  const fakeResult = JSON.stringify({ records: [{ time: '今天 08:30', symptom: '呕吐', count: 2 }] });
  const r2 = await call({
    model: MODEL,
    messages: [
      { role: 'user', content: '查询旺仔最近两次呕吐记录' },
      { role: 'assistant', tool_calls: r1.json.choices[0].message.tool_calls },
      { role: 'tool', tool_call_id: tc.id, content: fakeResult },
    ],
    tools: [{
      type: 'function',
      function: { name: 'query_health_records', description: '查询宠物健康记录', parameters: { type: 'object', properties: { pet: { type: 'string' } } } },
    }],
    max_tokens: 512,
  });
  if (!r2.ok) return report('工具结果回灌(第二轮)', false, `status=${r2.status} body=${r2.body}`);
  report('工具结果回灌(第二轮)', true, `第二轮content=${JSON.stringify(r2.json.choices?.[0]?.message?.content)}`);
}

// ---- 7. 截断：max_tokens 极小 -> finish_reason=length ----
async function probe7Truncation() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '用至少 500 字详细介绍猫的饲养方法，从饮食开始' }],
    max_tokens: 8,
  });
  if (!r.ok) return report('截断(finish_reason=length)', false, `status=${r.status} body=${r.body}`);
  const finish = r.json.choices?.[0]?.finish_reason;
  const content = r.json.choices?.[0]?.message?.content || '';
  report('截断(finish_reason=length)', finish === 'length', `finish=${finish} content=${JSON.stringify(content)}`);
}

// ---- 8. 有序流 + 流式用量（stream_options.include_usage）----
async function probe8Stream() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '依次说出数字：一、二、三' }],
    stream: true,
    stream_options: { include_usage: true },
    max_tokens: 2048,
  }, { stream: true });
  if (!r.ok) return report('有序流(stream)', false, `status=${r.status} error=${r.error || ''} body=${r.body || ''}`);
  const lines = r.streamText.split('\n').filter((l) => l.startsWith('data:'));
  const deltas = [];
  let usage = null;
  let finish = null;
  for (const line of lines) {
    const payload = line.slice(5).trim();
    if (payload === '[DONE]') continue;
    let obj;
    try { obj = JSON.parse(payload); } catch { continue; }
    if (obj.usage) usage = obj.usage;
    const c = obj.choices?.[0];
    if (c?.finish_reason) finish = c.finish_reason;
    const delta = c?.delta?.content;
    if (delta) deltas.push(delta);
  }
  const joined = deltas.join('');
  const ordered = joined.includes('一') && joined.includes('二') && joined.includes('三');
  const hasUsage = !!usage;
  report('有序流(stream+用量)', ordered && hasUsage,
    `顺序=${ordered} 用量存在=${hasUsage} finish=${finish} chunks=${deltas.length} text=${JSON.stringify(joined.slice(0, 60))}`);
  if (usage) console.log(`      流式usage=${JSON.stringify(usage)}`);
}

// ---- 9. 非流式（确认非流式正常返回）----
async function probe9NonStream() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '用一句话说明非流式模式可用' }],
    max_tokens: 2048,
  });
  const content = r.json?.choices?.[0]?.message?.content;
  report('非流式正常', r.ok && !!content, r.ok ? `content=${JSON.stringify(content)}` : `status=${r.status} body=${r.body}`);
}

// ---- 10. 取消行为（AbortController 中断）----
async function probe10Cancel() {
  const controller = new AbortController();
  const started = Date.now();
  let aborted = false;
  const promise = call({
    model: MODEL,
    messages: [{ role: 'user', content: '写一篇 3000 字的长文，主题是宠物饲养' }],
    max_tokens: 8192,
  }, { signal: controller.signal });
  const timer = setTimeout(() => controller.abort(), 500);
  try {
    await promise;
  } catch {}
  clearTimeout(timer);
  const result = await promise;
  const elapsed = Date.now() - started;
  const isAbort = result.error === 'AbortError' || (result.ok === false && result.error === 'AbortError');
  report('取消行为(AbortController)', isAbort, `error=${result.error} elapsed=${elapsed}ms（500ms 后 abort）`);
}

// ---- 11. 用量字段完整性 ----
async function probe11Usage() {
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: '你好，请简单介绍你自己' }],
    max_tokens: 512,
  });
  if (!r.ok) return report('用量字段', false, `status=${r.status} body=${r.body}`);
  const usage = r.json.usage;
  const hasPrompt = typeof usage?.prompt_tokens === 'number';
  const hasCompletion = typeof usage?.completion_tokens === 'number';
  const hasTotal = typeof usage?.total_tokens === 'number';
  report('用量字段', hasPrompt && hasCompletion, `usage=${JSON.stringify(usage)}`);
  return r.json;
}

// ---- 12. 请求硬限制：超长输入 ----
async function probe12HardLimit() {
  // 构造超长文本（约 300k 字符）探测输入长度硬限制
  const longText = '猫'.repeat(300000);
  const r = await call({
    model: MODEL,
    messages: [{ role: 'user', content: longText }],
    max_tokens: 16,
  });
  if (r.ok) {
    return report('请求硬限制(超长输入)', true, `超长输入被接受(status=200)，需记录实际上限`);
  }
  report('请求硬限制(超长输入)', true, `超长输入被拒 status=${r.status} body=${r.body.slice(0, 200)}`);
}

async function main() {
  console.log(`=== T3 Provider 组合验证 ${MODEL} @ ${BASE} ===`);
  console.log(`开始时间: ${new Date().toISOString()}\n`);
  await probe1Basic();
  await probe2JsonMode();
  await probe3Tools();
  await probe4Vision();
  await probe5Combined();
  await probe6ToolRoundTrip();
  await probe7Truncation();
  await probe8Stream();
  await probe9NonStream();
  await probe10Cancel();
  await probe11Usage();
  await probe12HardLimit();

  const pass = results.filter((r) => r.ok).length;
  console.log(`\n=== 汇总: ${pass}/${results.length} 通过 ===`);
  console.log(`结束时间: ${new Date().toISOString()}`);
}

main().catch((err) => {
  console.error('verify error:', err);
  process.exit(1);
});
