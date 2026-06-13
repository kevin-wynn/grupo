const jsonHeaders = { 'Content-Type': 'application/json' }

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    credentials: 'same-origin',
    ...init,
    headers: {
      ...jsonHeaders,
      ...(init?.headers ?? {}),
    },
  })
  if (!res.ok) {
    const body = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(body.error ?? res.statusText)
  }
  if (res.status === 204) {
    return undefined as T
  }
  return res.json()
}

export type Installation = {
  id: number
  account_login: string
  account_type: string
}

export type Deployment = {
  id: string
  project_id: string
  status: 'queued' | 'running' | 'success' | 'failed'
  commit_sha: string
  commit_message: string
  started_at?: string
  finished_at?: string
}

export type Project = {
  id: string
  name: string
  installation_id: number
  github_owner: string
  github_repo: string
  branch: string
  build_image: string
  build_command: string
  output_dir: string
  root_dir: string
  domain: string
  spa_fallback: boolean
  last_deployment?: Deployment
}

export type Me = {
  github_login: string
  github_user_id: number
  installations: Installation[]
  dev_mode?: boolean
}

export type Settings = {
  admin_domain: string
  data_dir: string
  github_configured: boolean
  skip_github_auth: boolean
  dev_mode: boolean
}

export type Repo = {
  id: number
  name: string
  full_name: string
}

export const api = {
  me: () => request<Me>('/api/me'),
  settings: () => request<Settings>('/api/settings'),
  projects: () => request<Project[]>('/api/projects'),
  project: (id: string) => request<Project>(`/api/projects/${id}`),
  createProject: (body: Record<string, unknown>) =>
    request<Project>('/api/projects', { method: 'POST', body: JSON.stringify(body) }),
  updateProject: (id: string, body: Record<string, unknown>) =>
    request<Project>(`/api/projects/${id}`, { method: 'PATCH', body: JSON.stringify(body) }),
  deleteProject: (id: string) =>
    request<void>(`/api/projects/${id}`, { method: 'DELETE' }),
  deploy: (id: string) =>
    request<Deployment>(`/api/projects/${id}/deploy`, { method: 'POST' }),
  deployments: (id: string) => request<Deployment[]>(`/api/projects/${id}/deployments`),
  deploymentLogs: (id: string) => fetch(`/api/deployments/${id}/logs`, { credentials: 'same-origin' }).then(r => r.text()),
  repos: (installationId: number) =>
    request<Repo[]>(`/api/github/repos?installation_id=${installationId}`),
  logout: () => request<void>('/auth/logout', { method: 'POST' }),
}

export function statusColor(status?: string) {
  switch (status) {
    case 'success':
      return 'text-emerald-400'
    case 'failed':
      return 'text-red-400'
    case 'running':
      return 'text-amber-400'
    default:
      return 'text-slate-400'
  }
}
