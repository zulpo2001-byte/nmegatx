export const tokenKey = 'nme_access_token'
export function token() { return localStorage.getItem(tokenKey) || '' }

export async function request(path, method = 'GET', body) {
  const headers = { 'Content-Type': 'application/json' }
  const t = token()
  if (t) headers.Authorization = `Bearer ${t}`
  const resp = await fetch(path, { method, headers, body: body ? JSON.stringify(body) : undefined })
  const json = await resp.json().catch(() => ({}))
  if (!json.ok) throw new Error(json.message || `HTTP ${resp.status}`)
  return json.data || {}
}
