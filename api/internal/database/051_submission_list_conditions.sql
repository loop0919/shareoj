-- The time a submission list row shows: the largest cpuTimeMs of the cases, as Result.summarizeUsage reads it. NULL when
-- no case has one, when cases is missing or not an array, or when result is NULL. A cpuTimeMs that is not a number is
-- skipped instead of failing the whole list.
CREATE FUNCTION submission_cpu_time_ms(result jsonb) RETURNS double precision
LANGUAGE sql IMMUTABLE AS $$
    SELECT max(CASE WHEN jsonb_typeof(c->'cpuTimeMs')='number' THEN (c->>'cpuTimeMs')::double precision END)
    FROM jsonb_array_elements(CASE WHEN jsonb_typeof(result->'cases')='array' THEN result->'cases' ELSE '[]'::jsonb END) AS c
$$;
