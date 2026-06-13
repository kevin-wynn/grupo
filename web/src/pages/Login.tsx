import { Navigate, useSearchParams } from 'react-router-dom'
import { useAuth } from '../auth/AuthContext'

const errors: Record<string, string> = {
  invalid_state: 'Sign-in expired. Please try again.',
  missing_code: 'GitHub did not return an authorization code.',
  oauth_exchange: 'Could not complete GitHub authorization.',
  github_user: 'Could not load your GitHub profile.',
  sync_installations: 'Signed in but could not sync GitHub App installations.',
}

export default function Login() {
  const { loading, status, user } = useAuth()
  const [params] = useSearchParams()
  const errorKey = params.get('error') ?? ''
  const next = params.get('next') ?? '/'

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center text-slate-400">
        Loading…
      </div>
    )
  }

  if (status?.skip_github_auth || status?.logged_in || user) {
    return <Navigate to={next.startsWith('/') ? next : '/'} replace />
  }

  const installHref = status?.github_app_slug
    ? '/auth/github/install'
    : undefined

  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-8 shadow-xl">
        <h1 className="text-2xl font-semibold text-white">Grupo</h1>
        <p className="mt-2 text-sm text-slate-400">
          Self-hosted static site deployment from GitHub.
        </p>

        {errorKey && (
          <p className="mt-4 rounded-lg border border-red-900/50 bg-red-950/40 px-3 py-2 text-sm text-red-300">
            {errors[errorKey] ?? 'Sign-in failed. Please try again.'}
          </p>
        )}

        {!status?.github_configured && (
          <p className="mt-4 rounded-lg border border-amber-900/50 bg-amber-950/40 px-3 py-2 text-sm text-amber-200">
            GitHub App is not configured on this instance. Set{' '}
            <code className="text-amber-100">GITHUB_APP_ID</code> and related env vars.
          </p>
        )}

        <a
          href="/auth/github"
          className="mt-8 inline-flex w-full items-center justify-center rounded-lg bg-white px-4 py-3 text-sm font-medium text-slate-900 hover:bg-slate-200"
        >
          Sign in with GitHub
        </a>

        {installHref && (
          <a
            href={installHref}
            className="mt-3 inline-flex w-full items-center justify-center rounded-lg border border-slate-600 px-4 py-3 text-sm font-medium text-slate-200 hover:border-slate-400"
          >
            Install GitHub App
          </a>
        )}

        <p className="mt-4 text-center text-xs text-slate-500">
          Install the Grupo GitHub App on your account or org, then sign in to manage projects.
        </p>
      </div>
    </div>
  )
}
