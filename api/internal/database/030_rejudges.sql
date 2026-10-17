-- A rejudge requeues a finished submission with a fresh attempt. Its request time orders,
-- expires and ages the new run instead of the original submission time.
ALTER TABLE submissions ADD COLUMN rejudged_at timestamptz;

-- Rejudges are sent only while few judgements are in flight.
CREATE INDEX submissions_in_flight ON submissions(dispatched_at) WHERE dispatched_at IS NOT NULL AND status <> 'DONE';
