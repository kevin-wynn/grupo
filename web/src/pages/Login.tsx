export default function Login() {
  return (
    <div className="flex min-h-screen items-center justify-center px-4">
      <div className="w-full max-w-md rounded-2xl border border-slate-800 bg-slate-900 p-8 shadow-xl">
        <h1 className="text-2xl font-semibold text-white">Grupo</h1>
        <p className="mt-2 text-sm text-slate-400">
          Self-hosted static site deployment from GitHub.
        </p>
        <a
          href="/auth/github"
          className="mt-8 inline-flex w-full items-center justify-center rounded-lg bg-white px-4 py-3 text-sm font-medium text-slate-900 hover:bg-slate-200"
        >
          Sign in with GitHub
        </a>
        <p className="mt-4 text-center text-xs text-slate-500">
          Dev mode with <code className="text-slate-400">GRUPO_SKIP_GITHUB_AUTH=true</code> skips login.
        </p>
      </div>
    </div>
  )
}
