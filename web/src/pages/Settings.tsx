import { useEffect, useState } from 'react'
import { api, type Settings } from '../api'

export default function SettingsPage() {
  const [settings, setSettings] = useState<Settings | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    api.settings()
      .then(setSettings)
      .catch(err => setError(err.message))
  }, [])

  if (error) return <p className="text-red-400">{error}</p>
  if (!settings) return <p className="text-slate-400">Loading…</p>

  return (
    <div className="max-w-2xl">
      <h1 className="text-2xl font-semibold text-white">Settings</h1>
      <p className="mt-1 text-sm text-slate-400">Instance configuration (read-only).</p>

      <dl className="mt-8 space-y-4">
        <Row label="Admin domain" value={settings.admin_domain} />
        <Row label="Data directory" value={settings.data_dir} mono />
        <Row label="GitHub App configured" value={settings.github_configured ? 'yes' : 'no'} />
        {settings.github_app_slug && (
          <Row label="GitHub App slug" value={settings.github_app_slug} />
        )}
        <Row label="Skip GitHub auth" value={settings.skip_github_auth ? 'yes' : 'no'} />
        <Row label="Dev mode" value={settings.dev_mode ? 'yes' : 'no'} />
      </dl>
    </div>
  )
}

function Row({ label, value, mono }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="rounded-xl border border-slate-800 bg-slate-900/50 px-4 py-4">
      <dt className="text-xs uppercase tracking-wide text-slate-500">{label}</dt>
      <dd className={`mt-1 text-slate-200 ${mono ? 'font-mono text-sm' : ''}`}>{value}</dd>
    </div>
  )
}
