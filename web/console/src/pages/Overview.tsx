import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { api, formatDate } from '../api'
import { EmptyState, InlineErrorState, PageHeaderBlock, StatCard, TableSkeleton } from '../ui'

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
  const [hoveredBucketIndex, setHoveredBucketIndex] = useState<number | null>(null)
  const [pinnedBucketIndex, setPinnedBucketIndex] = useState<number | null>(null)

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
      setHoveredBucketIndex(null)
      setPinnedBucketIndex(null)
    } else {
      setTimeseries([])
      setHoveredBucketIndex(null)
      setPinnedBucketIndex(null)
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
  const activeBucketIndex = timeseries.length === 0
    ? null
    : Math.min(pinnedBucketIndex ?? hoveredBucketIndex ?? (timeseries.length - 1), timeseries.length - 1)
  const activeBucket = activeBucketIndex == null ? null : timeseries[activeBucketIndex]
  const pinnedBucket = pinnedBucketIndex == null ? null : timeseries[Math.min(pinnedBucketIndex, timeseries.length - 1)]
  const scaleTicks = Array.from(new Set([maxCount, Math.ceil(maxCount / 2), 0])).sort((a, b) => b - a)
  const midpointBucket = timeseries.length > 2 ? timeseries[Math.floor((timeseries.length - 1) / 2)] : null

  return (
    <div>
      <PageHeaderBlock
        title="Overview"
        description="System-wide analytics and recent activity across tenants, approvals, and governed tool runs."
      />

      {error ? <InlineErrorState message={error} onRetry={() => void fetchOverview()} /> : null}

      <div className="stats-grid overview-stats-grid">
        <StatCard label="Total Events" value={overview.total_events.toLocaleString()} />
        <StatCard label="Allowed" value={overview.allow_count.toLocaleString()} tone="green" />
        <StatCard label="Denied" value={overview.deny_count.toLocaleString()} tone="red" />
        <StatCard label="Approval Required" value={overview.approve_count.toLocaleString()} tone="yellow" />
        <StatCard label="Pending Approvals" value={overview.pending_approvals.toLocaleString()} tone="yellow" />
        <StatCard label="Active Tenants" value={overview.active_tenants.toLocaleString()} tone="blue" />
      </div>

      {!loading && timeseries.length > 0 ? (
        <div className="detail-panel event-volume-panel">
          <h3>Event Volume</h3>
          <div className="event-volume-header">
            <div className="table-subtext">Hover to inspect a bucket, then click or press Enter/Space to pin it.</div>
            {activeBucket ? (
              <div className="event-volume-summary">
                <span className="event-volume-total">{activeBucket.total}</span>
                <span>
                  {activeBucket.total === 1 ? 'event' : 'events'} on {formatDate(activeBucket.bucket, 'date')}
                </span>
              </div>
            ) : null}
          </div>
          <div className="event-volume-layout">
            <div className="event-volume-scale">
              {scaleTicks.map(tick => (
                <span key={tick}>{tick}</span>
              ))}
            </div>
            <div className="event-volume-bars">
              {timeseries.map((bucket, index) => (
                <button
                  key={`${bucket.bucket}-${index}`}
                  type="button"
                  className={[
                    'event-volume-bar',
                    index === activeBucketIndex ? 'is-active' : '',
                    index === hoveredBucketIndex ? 'is-hovered' : '',
                    index === pinnedBucketIndex ? 'is-pinned' : '',
                  ].filter(Boolean).join(' ')}
                  title={`${formatDate(bucket.bucket, 'date')}: ${bucket.total} events`}
                  aria-label={`${bucket.total} events on ${formatDate(bucket.bucket, 'date')}`}
                  aria-pressed={index === pinnedBucketIndex}
                  onClick={() => setPinnedBucketIndex(index)}
                  onMouseEnter={() => setHoveredBucketIndex(index)}
                  onMouseLeave={() => setHoveredBucketIndex(null)}
                  onFocus={() => setHoveredBucketIndex(index)}
                  onBlur={() => setHoveredBucketIndex(null)}
                  style={{
                    height: `${((bucket.total || 0) / maxCount) * 100}%`,
                    minHeight: 6,
                  }}
                />
              ))}
            </div>
          </div>
          <div className="event-volume-axis">
            <span>{formatDate(timeseries[0].bucket, 'date')}</span>
            {midpointBucket ? <span>{formatDate(midpointBucket.bucket, 'date')}</span> : null}
            <span>{formatDate(timeseries[timeseries.length - 1].bucket, 'date')}</span>
          </div>
          {pinnedBucket ? (
            <div className="event-volume-selected">
              <div>
                <div className="event-volume-selected-label">Selected</div>
                <div className="event-volume-selected-value">
                  {formatDate(pinnedBucket.bucket, 'date')} — {pinnedBucket.total} {pinnedBucket.total === 1 ? 'event' : 'events'}
                </div>
              </div>
              <button className="btn btn-outline btn-sm" type="button" onClick={() => setPinnedBucketIndex(null)}>
                Clear selection
              </button>
            </div>
          ) : null}
        </div>
      ) : !loading ? (
        <div className="detail-panel">
          <EmptyState
            icon="◔"
            title="Not enough activity yet"
            description="The event volume chart will appear after governed tool calls start flowing through the gateway."
          />
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
                <td colSpan={5}>
                  <EmptyState
                    icon="◎"
                    title="No recent events yet"
                    description="Run the demo or submit a governed tool call to populate the dashboard and recent activity feed."
                  />
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
