-- Featured rounds start at 22:00 JST instead of 23:00 and still end at 22:00 the next day (ADR 0011).
-- Only the round not yet released moves, with its saved preview; released rounds keep their reveal_at.
UPDATE featured_previews p SET scheduled_at=p.scheduled_at-interval '1 hour'
FROM featured_schedule s
WHERE p.scheduled_at=s.next_at AND extract(hour FROM s.next_at AT TIME ZONE 'Asia/Tokyo')=23;
UPDATE featured_schedule SET next_at=next_at-interval '1 hour'
WHERE extract(hour FROM next_at AT TIME ZONE 'Asia/Tokyo')=23;
