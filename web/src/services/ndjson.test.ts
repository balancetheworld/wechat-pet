import type { RecursiveReader } from './ndjson'
import { expect, it, vi } from 'vitest'
import { NDJSONDecoder, read } from './ndjson'

it('decoder keeps partial JSON and parses multiple complete frames', () => {
  const decoder = new NDJSONDecoder<{ value: string }>()
  const bytes = new TextEncoder().encode('{"value":"旺仔"}\n{"value":"球球"}\n')
  const first = decoder.push(bytes.slice(0, 12))
  const second = decoder.push(bytes.slice(12))
  expect(first).toEqual([])
  expect(second).toEqual([{ value: '旺仔' }, { value: '球球' }])
  expect(decoder.finish()).toEqual([])
})

it('recursive read emits only complete JSON values', async () => {
  const chunks = [
    new TextEncoder().encode('{"sequence":1'),
    new TextEncoder().encode('}\n{"sequence":2}\n'),
  ]
  let index = 0
  const reader: RecursiveReader = {
    async read() {
      if (index >= chunks.length) {
        return { done: true, value: undefined }
      }
      const value = chunks[index]
      index++
      return { done: false, value }
    },
  }
  const values: Array<{ sequence: number }> = []
  await read(reader, new NDJSONDecoder<{ sequence: number }>(), incoming => values.push(...incoming))
  expect(values).toEqual([{ sequence: 1 }, { sequence: 2 }])
})

it('decoder falls back when TextDecoder is unavailable', () => {
  vi.stubGlobal('TextDecoder', undefined)
  const decoder = new NDJSONDecoder<{ value: string }>()
  const bytes = new TextEncoder().encode('{"value":"旺仔"}\n')
  expect(decoder.push(bytes.slice(0, 11))).toEqual([])
  expect(decoder.push(bytes.slice(11))).toEqual([{ value: '旺仔' }])
  vi.unstubAllGlobals()
})
