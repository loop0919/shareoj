ALTER TABLE contest_clarifications
    ALTER COLUMN owner_id DROP NOT NULL,
    ALTER COLUMN problem_id DROP NOT NULL,
    DROP CONSTRAINT contest_clarifications_question_check,
    ADD FOREIGN KEY (contest_id) REFERENCES contests(id),
    ADD CHECK (
        (owner_id IS NOT NULL AND problem_id IS NOT NULL AND char_length(question) BETWEEN 1 AND 4000)
        OR (owner_id IS NULL AND question = '' AND char_length(answer) BETWEEN 1 AND 4000
            AND answered_by IS NOT NULL AND answered_at IS NOT NULL AND published_at IS NOT NULL)
    );
