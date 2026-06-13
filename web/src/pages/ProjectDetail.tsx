import { useEffect, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { api, statusColor, type Deployment, type Project } from '../api'

export default function ProjectDetail() {
  const { id = '' } = useParams()
  const [project, setProject] = useState<Project | null>(null)
  const [deployments, setDeployments] = useState<Deployment[]>([])
  const [logs, setLogs] = useState('')
  const [selectedLog, setSelectedLog] = useState('')
  const [error, setError] = useState('')
  const [deploying, setDeploying] = useState(false)

  async function refresh() {
    const [p, d] = await Promise.all([api.project(id), api.deployments(id)])
    setProject(p)
    setDeployments(d)
  }

  useEffect(() => {
    refresh().catch(err => setError(err.message))
  }, [id])

  async function deployNow() {
    setDeploying(true)
    try {
      await api.deploy(id)
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Deploy failed')
    } finally {
      setDeploying(false)
    }
  }

  async function viewLogs(deploymentId: string) {
    setSelectedLog(deploymentId)
    const text = await api.deploymentLogs(deploymentId)
    setLogs(text)
  }

  async function removeProject() {
    if (!confirm('Delete this project and its deployments?')) return
    await api.deleteProject(id)
    window.location.href = '/'
  }

  if (error) return <p className="text-red-400">{error}</p>
  if (!project) return <p className="text-slate-400">Loading…</p>

  return (
    <div>
      <div className="flex flex-wrap items-start justify-between gap-4">
        <div>
          <p className="text-sm text-slate-500">
            <Link to="/" className="hover:text-slate-300">Projects</Link> / {project.name}
          </p>
          <h1 className="mt-1 text-2xl font-semibold text-white">{project.name}</h1>
          <p className="mt-2 text-sm text-slate-400">
            <a href={`http://${project.domain}:8080`} className="text-emerald-400 hover:underline" target="_blank" rel="noreferrer">
              {project.domain}
            </a>
            {' · '}{project.github_owner}/{project.github_repo} · {project.branch}
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={deployNow}
            disabled={deploying}
            className="rounded-lg bg-emerald-500 px-4 py-2 text-sm font-medium text-slate-950 hover:bg-emerald-400 disabled:opacity-50"
          >
            {deploying ? 'Deploying…' : 'Deploy now'}
          </button>
          <button
            onClick={removeProject}
            className="rounded-lg border border-red-900 px-4 py-2 text-sm text-red-300 hover:border-red-700"
          >
            Delete
          </button>
        </div>
      </div>

      <section className="mt-10">
        <h2 className="text-lg font-medium text-white">Build settings</h2>
        <dl className="mt-4 grid gap-3 text-sm md:grid-cols-2">
          <Item label="Image" value={project.build_image} />
          <Item label="Command" value={project.build_command} mono />
          <Item label="Output dir" value={project.output_dir} />
          <Item label="Root dir" value={project.root_dir || '—'} />
          <Item label="SPA fallback" value={project.spa_fallback ? 'yes' : 'no'} />
        </dl>
      </section>

      <section className="mt-10">
        <h2 className="text-lg font-medium text-white">Deployments</h2>
        <div className="mt-4 overflow-hidden rounded-xl border border-slate-800">
          <table className="min-w-full text-left text-sm">
            <thead className="bg-slate-900 text-slate-400">
              <tr>
                <th className="px-4 py-3 font-medium">Status</th>
                <th className="px-4 py-3 font-medium">Commit</th>
                <th className="px-4 py-3 font-medium">Started</th>
                <th className="px-4 py-3 font-medium"></th>
              </tr>
            </thead>
            <tbody>
              {deployments.map(d => (
                <tr key={d.id} className="border-t border-slate-800">
                  <td className={`px-4 py-3 font-medium uppercase ${statusColor(d.status)}`}>{d.status}</td>
                  <td className="px-4 py-3 text-slate-300">{d.commit_message || d.commit_sha || '—'}</td>
                  <td className="px-4 py-3 text-slate-400">{d.started_at ? new Date(d.started_at).toLocaleString() : '—'}</td>
                  <td className="px-4 py-3">
                    <button className="text-emerald-400 hover:underline" onClick={() => viewLogs(d.id)}>Logs</button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </section>

      {selectedLog && (
        <section className="mt-8">
          <h3 className="text-sm font-medium text-slate-300">Build log</h3>
          <pre className="mt-2 max-h-96 overflow-auto rounded-xl border border-slate-800 bg-black/40 p-4 text-xs text-slate-300">
            {logs || 'No logs yet.'}
          </pre>
        </section>
      )}
    </div>
  )
}

function Item({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-lg border border-slate-800 bg-slate-900/40 px-4 py-3">
      <dt className="text-xs uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className={`mt-1 text-slate-200 ${mono ? 'font-mono text-xs break-all' : ''}`}>{value}</dd>
    </div>
  )
}
