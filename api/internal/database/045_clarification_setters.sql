-- Co-setters answer private questions like the owner. Staff of the other problems still see only public ones.
CREATE OR REPLACE FUNCTION contest_clarification_manager(contest uuid, problem uuid, actor text) RETURNS boolean
LANGUAGE sql STABLE AS $$
    SELECT actor<>'' AND (contest_setter(contest, actor) OR can_manage_problem_directly(problem, actor))
$$;
