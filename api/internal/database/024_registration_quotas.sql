-- subject is a SHA-256 digest, so the table stores no raw IP or email address.
CREATE TABLE registration_quotas (
    subject text PRIMARY KEY CHECK (subject <> ''),
    full_at timestamptz NOT NULL
);
CREATE INDEX registration_quotas_full_at ON registration_quotas (full_at);
