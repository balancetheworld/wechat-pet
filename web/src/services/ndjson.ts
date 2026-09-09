export interface RecursiveReader {
  read: () => Promise<ReadableStreamReadResult<Uint8Array>>
}

interface StreamDecoder {
  decode: (input?: Uint8Array, options?: { stream?: boolean }) => string
}

class UTF8StreamDecoder implements StreamDecoder {
  private pending = new Uint8Array(0)

  decode(input = new Uint8Array(0), options?: { stream?: boolean }) {
    const bytes = new Uint8Array(this.pending.length + input.length)
    bytes.set(this.pending)
    bytes.set(input, this.pending.length)
    this.pending = new Uint8Array(0)
    let result = ''
    let index = 0
    while (index < bytes.length) {
      const first = bytes[index]
      let length = 1
      let codePoint = first
      let minimum = 0
      if (first >= 0xC2 && first <= 0xDF) {
        length = 2
        codePoint = first & 0x1F
        minimum = 0x80
      }
      else if (first >= 0xE0 && first <= 0xEF) {
        length = 3
        codePoint = first & 0x0F
        minimum = 0x800
      }
      else if (first >= 0xF0 && first <= 0xF4) {
        length = 4
        codePoint = first & 0x07
        minimum = 0x10000
      }
      else if (first >= 0x80) {
        result += '\uFFFD'
        index++
        continue
      }
      if (index + length > bytes.length) {
        if (options?.stream) {
          this.pending = bytes.slice(index)
          break
        }
        result += '\uFFFD'
        index++
        continue
      }
      let valid = true
      for (let offset = 1; offset < length; offset++) {
        const value = bytes[index + offset]
        if ((value & 0xC0) !== 0x80) {
          valid = false
          break
        }
        codePoint = (codePoint << 6) | (value & 0x3F)
      }
      if (!valid || codePoint < minimum || codePoint > 0x10FFFF || (codePoint >= 0xD800 && codePoint <= 0xDFFF)) {
        result += '\uFFFD'
        index++
        continue
      }
      result += String.fromCodePoint(codePoint)
      index += length
    }
    return result
  }
}

function createStreamDecoder(): StreamDecoder {
  return typeof TextDecoder === 'undefined' ? new UTF8StreamDecoder() : new TextDecoder()
}

export class NDJSONDecoder<T> {
  private buffer = ''
  private readonly decoder = createStreamDecoder()

  push(chunk: ArrayBuffer | Uint8Array | string) {
    if (typeof chunk === 'string') {
      this.buffer += chunk
    }
    else {
      const bytes = chunk instanceof Uint8Array ? chunk : new Uint8Array(chunk)
      this.buffer += this.decoder.decode(bytes, { stream: true })
    }
    return this.takeLines()
  }

  finish() {
    this.buffer += this.decoder.decode()
    const values = this.takeLines()
    const remaining = this.buffer.trim()
    this.buffer = ''
    if (remaining) {
      values.push(JSON.parse(remaining) as T)
    }
    return values
  }

  private takeLines() {
    const lines = this.buffer.split('\n')
    this.buffer = lines.pop() ?? ''
    const values: T[] = []
    for (const line of lines) {
      const value = line.trim()
      if (value) {
        values.push(JSON.parse(value) as T)
      }
    }
    return values
  }
}

export async function read<T>(reader: RecursiveReader, decoder: NDJSONDecoder<T>, onValues: (values: T[]) => void): Promise<void> {
  const result = await reader.read()
  if (result.done) {
    const values = decoder.finish()
    if (values.length) {
      onValues(values)
    }
    return
  }
  const values = decoder.push(result.value)
  if (values.length) {
    onValues(values)
  }
  return read(reader, decoder, onValues)
}
