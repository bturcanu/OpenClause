import { useEffect, useState } from 'react'
import { Routes, Route, Navigate, NavLink, useNavigate } from 'react-router-dom'
import { api, clearStoredAuth, readJSONResponse } from './api'
import { InlineErrorState } from './ui'
import Login from './pages/Login'
import Overview from './pages/Overview'
import Approvals from './pages/Approvals'
import Events from './pages/Events'
import EventDetail from './pages/EventDetail'
import Tenants from './pages/Tenants'
import TenantDetail from './pages/TenantDetail'
import Sessions from './pages/Sessions'
import SessionTimeline from './pages/SessionTimeline'
import Policies from './pages/Policies'
import Alerts from './pages/Alerts'
import Connectors from './pages/Connectors'
import Users from './pages/Users'
import InviteAccept from './pages/InviteAccept'
import PasswordReset from './pages/PasswordReset'
import SetupWizard from './pages/SetupWizard'

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = localStorage.getItem('oc_token')
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

function Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()

  async function handleLogout() {
    try {
      await api.post('/auth/logout')
    } catch {
      // Best-effort server-side logout; client state is always cleared below.
    }
    clearStoredAuth()
    navigate('/login')
  }

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <h1>OpenClause</h1>
          <span>Console</span>
        </div>
        <nav className="sidebar-nav">
          <div className="sidebar-section">Main</div>
          <NavLink to="/" end>
            <span className="nav-icon">◉</span> Overview
          </NavLink>
          <NavLink to="/approvals">
            <span className="nav-icon">✓</span> Approvals
          </NavLink>
          <NavLink to="/events">
            <span className="nav-icon">▤</span> Audit Trail
          </NavLink>

          <div className="sidebar-section">Manage</div>
          <NavLink to="/tenants">
            <span className="nav-icon">⊞</span> Tenants
          </NavLink>
          <NavLink to="/users">
            <span className="nav-icon">⌁</span> Users
          </NavLink>
          <NavLink to="/sessions">
            <span className="nav-icon">↻</span> Sessions
          </NavLink>
          <NavLink to="/policies">
            <span className="nav-icon">☰</span> Policies
          </NavLink>

          <div className="sidebar-section">System</div>
          <NavLink to="/alerts">
            <span className="nav-icon">⚠</span> Alerts
          </NavLink>
          <NavLink to="/connectors">
            <span className="nav-icon">⧉</span> Connectors
          </NavLink>
        </nav>
        <div className="sidebar-footer">
          <button onClick={handleLogout}>Sign out</button>
        </div>
      </aside>
      <main className="main-content">{children}</main>
    </div>
  )
}

export default function App() {
  const [setupState, setSetupState] = useState<'checking' | 'initialized' | 'not_initialized' | 'error'>('checking')
  const [setupError, setSetupError] = useState('')
  const [setupCheckNonce, setSetupCheckNonce] = useState(0)

  useEffect(() => {
    let cancelled = false
    async function check() {
      try {
        if (!cancelled) {
          setSetupState('checking')
          setSetupError('')
        }
        const resp = await fetch('/api/setup/status', { method: 'GET', headers: { 'Content-Type': 'application/json' } })
        const data = await readJSONResponse(resp)
        if (cancelled) return
        if (!resp.ok) {
          setSetupError(data?.message || data?.error || 'Failed to check setup status. Verify console-api is reachable and try again.')
          setSetupState('error')
          return
        }
        setSetupState(data?.initialized ? 'initialized' : 'not_initialized')
      } catch (err) {
        if (!cancelled) {
          setSetupError(err instanceof Error ? err.message : 'Failed to check setup status. Verify console-api is reachable and try again.')
          setSetupState('error')
        }
      }
    }
    void check()
    return () => {
      cancelled = true
    }
  }, [setupCheckNonce])

  if (setupState === 'checking') {
    return <div className="loading">Checking setup…</div>
  }

  if (setupState === 'error') {
    return (
      <div className="auth-page">
        <div className="page-header">
          <h2>Console Setup</h2>
          <p>We couldn’t confirm whether this instance is initialized yet.</p>
        </div>
        <InlineErrorState message={setupError || 'Failed to check setup status.'} onRetry={() => setSetupCheckNonce(value => value + 1)} />
      </div>
    )
  }

  if (setupState === 'not_initialized') {
    return <SetupWizard onInitialized={() => setSetupState('initialized')} />
  }

  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/invite/accept" element={<InviteAccept />} />
      <Route path="/reset" element={<PasswordReset />} />
      <Route
        path="/*"
        element={
          <RequireAuth>
            <Layout>
              <Routes>
                <Route path="/" element={<Overview />} />
                <Route path="/approvals" element={<Approvals />} />
                <Route path="/events" element={<Events />} />
                <Route path="/events/:eventId" element={<EventDetail />} />
                <Route path="/tenants" element={<Tenants />} />
                <Route path="/tenants/:id" element={<TenantDetail />} />
                <Route path="/users" element={<Users />} />
                <Route path="/sessions" element={<Sessions />} />
                <Route path="/sessions/:id" element={<SessionTimeline />} />
                <Route path="/policies" element={<Policies />} />
                <Route path="/alerts" element={<Alerts />} />
                <Route path="/connectors" element={<Connectors />} />
              </Routes>
            </Layout>
          </RequireAuth>
        }
      />
    </Routes>
  )
}
