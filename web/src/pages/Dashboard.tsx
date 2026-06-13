import { Link } from 'react-router-dom'
import { useEffect, useState } from 'react'
import { api, statusColor, type Project } from '../api'

export default function Dashboard() {
  const [projects, setProjects] = useState<Project[]>([])
  const [error, setError] = useState('')

  useEffect(() => {
    api.projects()
      .then(setProjects)
      .catch(err => setError(err.message))
  }, [])

  return (
    <div>
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-white">Projects</h1>
          <p className="mt-1 text-sm text-slate-400">Static sites deployed from GitHub.</p>
        </div>
        <Link
          to="/projects/new"
          className="rounded-lg bg-emerald-500 px-4 py-2 text-sm font-medium text-slate-950 hover:bg-emerald-400"
        >
          New project
        </Link>
      </div>

      {error && <p className="mt-4 text-sm text-red-400">{error}</p>}

      <div className="mt-8 grid gap-4 md:grid-cols-2">
        {projects.map(project => (
          <Link
            key={project.id}
            to={`/projects/${project.id}`}
            className="rounded-xl border border-slate-800 bg-slate-900/60 p-5 transition hover:border-slate-600"
          >
            <div className="flex items-start justify-between gap-3">
              <div>
                <h2 className="text-lg font-medium text-white">{project.name}</h2>
                <p className="mt-1 text-sm text-slate-400">{project.domain}</p>
              </div>
              <span className={`text-xs font-medium uppercase ${statusColor(project.last_deployment?.status)}`}>
                {project.last_deployment?.status ?? 'none'}
              </span>
            </div>
            <p className="mt-4 text-xs text-slate-500">
              {project.github_owner}/{project.github_repo} · {project.branch}
            </p>
            <p className="mt-3 text-sm text-emerald-400">Open site →</p>
          </Link>
        ))}
      </div>

      {projects.length === 0 && !error && (
        <div className="mt-12 rounded-xl border border-dashed border-slate-800 p-10 text-center text-slate-400">
          No projects yet. Create one or run <code className="text-slate-300">make seed</code> in dev.
        </div>
      )}
    </div>
  )
}
