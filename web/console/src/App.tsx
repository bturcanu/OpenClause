import { Routes, Route, Navigate, NavLink, useNavigate } from 'react-router-dom'
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

function RequireAuth({ children }: { children: React.ReactNode }) {
  const token = localStorage.getItem('oc_token')
  if (!token) return <Navigate to="/login" replace />
  return <>{children}</>
}

function Layout({ children }: { children: React.ReactNode }) {
  const navigate = useNavigate()

  function handleLogout() {
    localStorage.removeItem('oc_token')
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
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
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
