import { beforeEach, vi } from 'vitest'

class MemoryStorage implements Storage {
  private readonly data = new Map<string, string>()

  get length() {
    return this.data.size
  }

  clear() {
    this.data.clear()
  }

  getItem(key: string) {
    return this.data.get(key) ?? null
  }

  key(index: number) {
    return Array.from(this.data.keys())[index] ?? null
  }

  removeItem(key: string) {
    this.data.delete(key)
  }

  setItem(key: string, value: string) {
    this.data.set(key, String(value))
  }
}

class NoopBroadcastChannel {
  readonly name: string

  constructor(name: string) {
    this.name = name
  }

  addEventListener() {}
  close() {}
  postMessage() {}
  removeEventListener() {}
}

const storage = new MemoryStorage()

vi.stubGlobal('localStorage', storage)
vi.stubGlobal('BroadcastChannel', NoopBroadcastChannel)

beforeEach(() => {
  storage.clear()
})