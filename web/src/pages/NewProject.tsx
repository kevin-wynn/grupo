import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, type Installation, type Repo } from '../api'

const imagePresets = [
  { label: 'Node.js 22', value: 'node:22-bookworm' },
  { label: 'Node.js 20', value: 'node:20-bookworm' },
  { label: 'Static (Alpine)', value: 'alpine:3.20' },
]

export default function NewProject() {
  const navigate = useNavigate()
  const [installations, setInstallations] = useState<Installation[]>([])
  const [repos, setRepos] = useState<Repo[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const [form, setForm] = useState({
    name: '',
    installation_id: 0,
    github_owner: '',
    github_repo: '',
    branch: 'main',
    build_image: 'node:22-bookworm',
    build_command: 'npm ci && npm run build',
    output_dir: 'dist',
    root_dir: '',
    domain: '',
    spa_fallback: true,
  })

  useEffect(() => {
    api.me().then(me => {
      setInstallations(me.installations)
      if (me.installations[0]) {
        setForm(f => ({ ...f, installation_id: me.installations[0].id }))
      }
    })
  }, [])

  useEffect(() => {
    if (form.installation_id > 0) {
      api.repos(form.installation_id).then(setRepos).catch(() => setRepos([]))
    }
  }, [form.installation_id])

  function selectRepo(fullName: string) {
    const [owner, repo] = fullName.split('/')
    setForm(f => ({
      ...f,
      github_owner: owner ?? '',
      github_repo: repo ?? '',
      name: f.name || repo || '',
      domain: f.domain || `${repo}.mygrupo.dev`,
    }))
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError('')
    try {
      const project = await api.createProject(form)
      navigate(`/projects/${project.id}`)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create project')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-semibold text-white">New project</h1>
      <p className="mt-1 text-sm text-slate-400">Connect a GitHub repo and configure the build.</p>

      <form onSubmit={submit} className="mt-8 space-y-5">
        {installations.length > 1 && (
          <Field label="Installation">
            <select
              className="input"
              value={form.installation_id}
              onChange={e => setForm(f => ({ ...f, installation_id: Number(e.target.value) }))}
            >
              {installations.map(i => (
                <option key={i.id} value={i.id}>{i.account_login}</option>
              ))}
            </select>
          </Field>
        )}

        <Field label="Repository">
          <select
            className="input"
            value={form.github_owner && form.github_repo ? `${form.github_owner}/${form.github_repo}` : ''}
            onChange={e => selectRepo(e.target.value)}
          >
            <option value="">Select a repository</option>
            {repos.map(r => (
              <option key={r.id} value={r.full_name}>{r.full_name}</option>
            ))}
          </select>
        </Field>

        <Field label="Name">
          <input className="input" value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} required />
        </Field>

        <Field label="Branch">
          <input className="input" value={form.branch} onChange={e => setForm(f => ({ ...f, branch: e.target.value }))} />
        </Field>

        <Field label="Build image">
          <select
            className="input"
            value={form.build_image}
            onChange={e => setForm(f => ({ ...f, build_image: e.target.value }))}
          >
            {imagePresets.map(p => <option key={p.value} value={p.value}>{p.label}</option>)}
          </select>
          <input
            className="input mt-2"
            placeholder="Custom image"
            value={form.build_image}
            onChange={e => setForm(f => ({ ...f, build_image: e.target.value }))}
          />
        </Field>

        <Field label="Build command">
          <input className="input font-mono text-sm" value={form.build_command} onChange={e => setForm(f => ({ ...f, build_command: e.target.value }))} required />
        </Field>

        <div className="grid gap-4 md:grid-cols-2">
          <Field label="Output directory">
            <input className="input" value={form.output_dir} onChange={e => setForm(f => ({ ...f, output_dir: e.target.value }))} />
          </Field>
          <Field label="Root directory (monorepo)">
            <input className="input" value={form.root_dir} onChange={e => setForm(f => ({ ...f, root_dir: e.target.value }))} placeholder="optional" />
          </Field>
        </div>

        <Field label="Domain">
          <input className="input" value={form.domain} onChange={e => setForm(f => ({ ...f, domain: e.target.value }))} required />
        </Field>

        <label className="flex items-center gap-2 text-sm text-slate-300">
          <input type="checkbox" checked={form.spa_fallback} onChange={e => setForm(f => ({ ...f, spa_fallback: e.target.checked }))} />
          SPA fallback (unknown paths → index.html)
        </label>

        {error && <p className="text-sm text-red-400">{error}</p>}

        <button
          type="submit"
          disabled={loading}
          className="rounded-lg bg-emerald-500 px-4 py-2 text-sm font-medium text-slate-950 hover:bg-emerald-400 disabled:opacity-50"
        >
          {loading ? 'Creating…' : 'Create project'}
        </button>
      </form>
    </div>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-sm text-slate-400">{label}</span>
      {children}
    </label>
  )
}
