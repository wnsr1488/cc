import { ElMessage } from 'element-plus'

import { clearSession, getToken } from '@/stores/auth'

const baseURL = import.meta.env.VITE_API_BASE_URL || ''

export interface ApiError {
  error: string
}

export async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const headers = new Headers(options.headers)
  const token = getToken()
  if (token) {
    headers.set('Authorization', `Bearer ${token}`)
  }
  if (options.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }

  const response = await fetch(`${baseURL}${path}`, {
    ...options,
    headers,
  })

  if (response.status === 401) {
    clearSession()
    window.location.href = '/login'
    throw new Error('登录已过期，请重新登录')
  }

  if (response.status === 204) {
    return undefined as T
  }

  const data = (await response.json().catch(() => ({}))) as ApiError | T
  if (!response.ok) {
    const message =
      typeof data === 'object' && data !== null && 'error' in data
        ? String(data.error)
        : `请求失败：${response.status}`
    ElMessage.error(message)
    throw new Error(message)
  }
  return data as T
}

export function jsonBody(payload: unknown): RequestInit {
  return { body: JSON.stringify(payload) }
}
