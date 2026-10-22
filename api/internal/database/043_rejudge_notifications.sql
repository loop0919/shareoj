-- A rejudge notifies the submitters of the problem. It names the problem only, so notifications_subject is unchanged.
ALTER TABLE notifications DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications ADD CONSTRAINT notifications_kind_check CHECK (kind IN ('favorite','first_accept','tester','user_editorial','contest_setter','clarification_question','clarification_answer','clarification_published','contest_results','rejudge'));
