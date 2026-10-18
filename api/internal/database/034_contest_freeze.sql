-- ADR 0023: freeze the public standings for the last freeze_minutes of a contest. NULL means no freeze.
-- revealed_at is when a setter or operator revealed the results of a frozen contest; it is never cleared.
ALTER TABLE contests ADD COLUMN freeze_minutes integer CHECK (freeze_minutes > 0);
ALTER TABLE contests ADD COLUMN revealed_at timestamptz;

-- When the results become final and public: the end without a freeze, the reveal with one.
-- NULL means that moment has not come yet.
CREATE OR REPLACE FUNCTION contest_results_final_at(c contests) RETURNS timestamptz
LANGUAGE sql STABLE AS $$
    SELECT CASE WHEN c.freeze_minutes IS NULL THEN c.ends_at ELSE c.revealed_at END
$$;

-- Submissions created in [freeze_at, ends_at) are the frozen ones; NULL without a freeze.
CREATE FUNCTION contest_freeze_at(c contests) RETURNS timestamptz
LANGUAGE sql STABLE AS $$
    SELECT c.ends_at-make_interval(mins => c.freeze_minutes)
$$;

-- Whether anyone may see a contest submission. Staff runs from before the start stay private, and frozen
-- submissions stay private until the results are final; the others, including practice after the end,
-- become public at the end.
CREATE OR REPLACE FUNCTION contest_submission_public(c contests, submitted_at timestamptz) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT submitted_at>=c.starts_at AND (contest_results_final(c) OR (c.ends_at<=statement_timestamp()
        AND NOT COALESCE(submitted_at>=contest_freeze_at(c) AND submitted_at<c.ends_at, false)))
$$;
