import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'
import { InlineErrorState, PageHeaderBlock, StatCard, TableSkeleton } from '../ui'

interface OverviewData {
  total_events: number
  allow_count: number
  deny_count: number
  approve_count: number
  pending_approvals: number
  active_tenants: number
}

interface TimeseriesBucket {
  bucket: string
  total: number
}

interface Event {
  event_id: string
  tool: string
  action: string
  decision: string
  risk_score: number
  received_at: string
  tenant_id: string
}

const EMPTY_OVERVIEW: OverviewData = {
  total_events: 0,
  allow_count: 0,
  deny_count: 0,
  approve_count: 0,
  pending_approvals: 0,
  active_tenants: 0,
}

export default function Overview() {
  const [overview, setOverview] = useState<OverviewData>(EMPTY_OVERVIEW)
  const [timeseries, setTimeseries] = useState<TimeseriesBucket[]>([])
  const [events, setEvents] = useState<Event[]>([])
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)

  async function fetchOverview() {
    setLoading(true)
    setError('')

    const [overviewResult, timeseriesResult, eventsResult] = await Promise.allSettled([
      api.get('/admin/analytics/overview'),
      api.get('/admin/analytics/timeseries'),
      api.get('/admin/events?limit=10'),
    ])

    const failures: string[] = []

    if (overviewResult.status === 'fulfilled') {
      setOverview((overviewResult.value as OverviewData) || EMPTY_OVERVIEW)
    } else {
      setOverview(EMPTY_OVERVIEW)
      failures.push('overview metrics')
    }

    if (timeseriesResult.status === 'fulfilled') {
      const nextTimeseries = Array.isArray(timeseriesResult.value)
        ? (timeseriesResult.value as TimeseriesBucket[])
        : ((timeseriesResult.value as { buckets?: TimeseriesBucket[] })?.buckets || [])
      setTimeseries(nextTimeseries)
    } else {
      setTimeseries([])
      failures.push('event volume chart')
    }

    if (eventsResult.status === 'fulfilled') {
      const nextEvents = Array.isArray(eventsResult.value)
        ? (eventsResult.value as Event[])
        : ((eventsResult.value as { events?: Event[] })?.events || [])
      setEvents(nextEvents)
    } else {
      setEvents([])
      failures.push('recent events')
    }

    if (failures.length > 0) {
      setError(`Some dashboard data could not be loaded: ${failures.join(', ')}.`)
    }

    setLoading(false)
  }

  useEffect(() => {
    void fetchOverview()
  }, [])

  const maxCount = Math.max(...timeseries.map(bucket => bucket.total || 0), 1)

  return (
    <div>
      <PageHeaderBlock
        title="Overview"
        description="System-wide analytics and recent activity across tenants, approvals, and governed tool runs."
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchOverview()} /> : null}

      <div className="stats-grid">
        <StatCard label="Total Events" value={overview.total_events.toLocaleString()} />
        <StatCard label="Allowed" value={overview.allow_count.toLocaleString()} tone="green" />
        <StatCard label="Denied" value={overview.deny_count.toLocaleString()} tone="red" />
        <StatCard label="Approval Required" value={overview.approve_count.toLocaleString()} tone="yellow" />
        <StatCard label="Pending Approvals" value={overview.pending_approvals.toLocaleString()} tone="yellow" />
        <StatCard label="Active Tenants" value={overview.active_tenants.toLocaleString()} tone="blue" />
      </div>

      {!loading && timeseries.length > 0 ? (
        <div className="detail-panel">
          <h3>Event Volume</h3>
          <div style={{ display: 'flex', alignItems: 'flex-end', gap: 2, height: 120, padding: '12px 0' }}>
            {timeseries.map((bucket, index) => (
              <div
                key={`${bucket.bucket}-${index}`}
                title={`${bucket.bucket}: ${bucket.total}`}
                style={{
                  flex: 1,
                  background: 'linear-gradient(180deg, #0f766e, #0b5f58)',
                  borderRadius: '6px 6px 0 0',
                  height: `${((bucket.total || 0) / maxCount) * 100}%`,
                  minHeight: 4,
                  transition: 'height 0.3s',
                }}
              />
            ))}
          </div>
          <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, color: '#746250' }}>
            <span>{formatDate(timeseries[0].bucket, 'date')}</span>
            <span>{formatDate(timeseries[timeseries.length - 1].bucket, 'date')}</span>
          </div>
        </div>
      ) : null}

      <div className="section-title">Recent Events</div>
      <div className="table-container table-sticky">
        <table>
          <thead>
            <tr>
              <th>Tool</th>
              <th>Action</th>
              <th>Decision</th>
              <th>Risk</th>
              <th>Time</th>
            </tr>
          </thead>
          <tbody>
            {loading ? (
              <TableSkeleton columns={5} rows={6} />
            ) : events.length === 0 ? (
              <tr>
                <td colSpan={5} style={{ textAlign: 'center', color: '#746250', padding: 28 }}>
                  No events yet. Run the demo or submit a tool call to populate the dashboard.
                </td>
              </tr>
            ) : (
              events.map(event => (
                <tr key={event.event_id}>
                  <td className="table-primary">
                    <Link to={`/events/${event.event_id}`}>{event.tool}</Link>
                    <div className="table-subtext">{event.tenant_id}</div>
                  </td>
                  <td>{event.action}</td>
                  <td><span className={`badge badge-${event.decision}`}>{event.decision}</span></td>
                  <td>{event.risk_score}</td>
                  <td>{formatDate(event.received_at)}</td>
                </tr>
              ))
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
