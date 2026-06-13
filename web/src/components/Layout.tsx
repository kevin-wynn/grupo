import { Link, Outlet } from 'react-router-dom'
import { api } from '../api'
import { useEffect, useState } from 'react'

export default function Layout() {
  const [login, setLogin] = useState('')

  useEffect(() => {
    api.me().then(me => setLogin(me.github_login)).catch(() => setLogin(''))
  }, [])

  return (
    <div className="min-h-screen">
      <header className="border-b border-slate-800 bg-slate-900/80 backdrop-blur">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-4 py-4">
          <div className="flex items-center gap-6">
            <Link to="/" className="text-lg font-semibold tracking-tight text-white">
              Grupo
            </Link>
            <nav className="flex gap-4 text-sm text-slate-300">
              <Link to="/" className="hover:text-white">Dashboard</Link>
              <Link to="/projects/new" className="hover:text-white">New project</Link>
              <Link to="/settings" className="hover:text-white">Settings</Link>
            </nav>
          </div>
          <div className="flex items-center gap-3 text-sm text-slate-400">
            {login && <span>{login}</span>}
            <button
              className="rounded-md border border-slate-700 px-3 py-1 hover:border-slate-500"
              onClick={() => api.logout().then(() => window.location.href = '/login')}
            >
              Log out
            </button>
          </div>
        </div>
      </header>
      <main className="mx-auto max-w-6xl px-4 py-8">
        <Outlet />
      </main>
    </div>
  )
}
