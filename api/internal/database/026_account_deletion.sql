-- A deleted account keeps its row, so public problems, contests and submissions keep an author,
-- but its identity is replaced and its access tokens are refused until they expire.
ALTER TABLE user_profiles ADD COLUMN deleted_at timestamptz;
