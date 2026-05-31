const tokenKey = 'cc_panel_token'
const userKey = 'cc_panel_user'

export interface CurrentUser {
  id: number
  username: string
  role: string
  created_at: string
}

export function getToken(): string {
  return localStorage.getItem(tokenKey) ?? ''
}

export function setSession(token: string, user: CurrentUser) {
  localStorage.setItem(tokenKey, token)
  localStorage.setItem(userKey, JSON.stringify(user))
}

export function clearSession() {
  localStorage.removeItem(tokenKey)
  localStorage.removeItem(userKey)
}

export function getCurrentUser(): CurrentUser | null {
  const raw = localStorage.getItem(userKey)
  if (!raw) return null
  try {
    return JSON.parse(raw) as CurrentUser
  } catch {
    clearSession()
    return null
  }
}

export function isAuthenticated(): boolean {
  return getToken() !== ''
}
