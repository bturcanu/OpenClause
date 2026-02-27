-- Ensure chain traversal uses a monotonic insertion-order key.
ALTER TABLE tool_events
    ADD COLUMN IF NOT EXISTS event_seq BIGINT;

CREATE SEQUENCE IF NOT EXISTS tool_events_event_seq_seq;

ALTER SEQUENCE tool_events_event_seq_seq
    OWNED BY tool_events.event_seq;

ALTER TABLE tool_events
    ALTER COLUMN event_seq SET DEFAULT nextval('tool_events_event_seq_seq');

-- Backfill existing rows deterministically for legacy datasets.
WITH ordered AS (
    SELECT event_id,
           ROW_NUMBER() OVER (ORDER BY received_at ASC, event_id ASC) AS rn
    FROM tool_events
    WHERE event_seq IS NULL
)
UPDATE tool_events t
SET event_seq = ordered.rn
FROM ordered
WHERE t.event_id = ordered.event_id;

SELECT setval(
    'tool_events_event_seq_seq',
    GREATEST(COALESCE((SELECT MAX(event_seq) FROM tool_events), 0), 1),
    true
);

ALTER TABLE tool_events
    ALTER COLUMN event_seq SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_tool_events_event_seq
    ON tool_events(event_seq);

ALTER TABLE evidence_archive_checkpoints
    ADD COLUMN IF NOT EXISTS last_event_seq BIGINT NOT NULL DEFAULT 0;
