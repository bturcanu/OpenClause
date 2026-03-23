import { vi } from 'vitest'
import { api } from '../api'

type Matcher = string | RegExp | ((path: string) => boolean)
type Resolver<T> = T | ((path: string) => T | Promise<T>)

function matches(path: string, matcher: Matcher) {
  if (typeof matcher === 'string') return path === matcher
  if (matcher instanceof RegExp) return matcher.test(path)
  return matcher(path)
}

async function resolveValue<T>(path: string, value: Resolver<T>) {
  return typeof value === 'function'
    ? await (value as (path: string) => T | Promise<T>)(path)
    : value
}

export function mockApiGet(routes: Array<[Matcher, Resolver<unknown>]>) {
  return vi.spyOn(api, 'get').mockImplementation(async (path: string) => {
    for (const [matcher, value] of routes) {
      if (matches(path, matcher)) return resolveValue(path, value)
    }
    throw new Error(`Unhandled api.get call for ${path}`)
  })
}

export function mockApiBlob(routes: Array<[Matcher, Resolver<Blob>]>) {
  return vi.spyOn(api, 'getBlob').mockImplementation(async (path: string) => {
    for (const [matcher, value] of routes) {
      if (matches(path, matcher)) return resolveValue(path, value)
    }
    throw new Error(`Unhandled api.getBlob call for ${path}`)
  })
}

export function stubMutableApi() {
  vi.spyOn(api, 'post').mockResolvedValue({})
  vi.spyOn(api, 'put').mockResolvedValue({})
  vi.spyOn(api, 'delete').mockResolvedValue({})
  vi.spyOn(api, 'getBlob').mockResolvedValue(new Blob(['ok'], { type: 'text/plain' }))
  vi.spyOn(api, 'unauthPost').mockResolvedValue({})
}
