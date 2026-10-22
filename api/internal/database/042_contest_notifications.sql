-- Clarifications and contest results notify through the bell. Like contest_setter, they name a contest and no problem.
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('favorite','first_accept','tester','user_editorial','contest_setter','clarification_question','clarification_answer','clarification_published','contest_results'));
ALTER TABLE notifications DROP CONSTRAINT notifications_subject;
ALTER TABLE notifications ADD CONSTRAINT notifications_subject CHECK (
    CASE WHEN kind IN ('contest_setter','clarification_question','clarification_answer','clarification_published','contest_results')
    THEN problem_id IS NULL AND contest_id IS NOT NULL ELSE problem_id IS NOT NULL AND contest_id IS NULL END
);
