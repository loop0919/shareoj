import base64
import os
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock, patch

from test_runner import host, sandbox, RUNTIMES
import interactive

RESULT = b'<?xml version="1.0" encoding="windows-1251"?><result outcome = "%s"%s>%s</result>\n'


def testlib_result(outcome, message=b'', points=None, pctype=None):
    attributes = (b' points = "%s"' % points if points is not None else b'') + (b' pctype = "%s"' % pctype if pctype is not None else b'')
    return RESULT % (outcome, attributes, message)


def scoring_job(mode='groups', groups=(('a', 40), ('b', 60)), members=(['a'], ['b']), **changes):
    job = dict(runtime='cpp17-isolate', runtimeDigest='sha256:test', source='submitted source',
               memoryLimitMb=512, timeLimitMs=1000,
               cases=[dict(name=f'c{i}', input=str(i), output='', isSample=False,
                           groups=list(member) if isinstance(member, list) else member)
                      for i, member in enumerate(members)],
               scoring=dict(mode=mode, groups=[dict(name=name, points=points) for name, points in groups]))
    if mode == 'aggregate':
        job['scoring']['aggregator'] = dict(runtime='cpp17', source='aggregator source')
    job.update(changes)
    return job


def finished(verdict='AC', **changes):
    reply = dict(status='', oom=False, overflow=False, exitCode=0, signal=0, cpuTimeMs=1, wallTimeMs=2, memoryBytes=1024,
                 output=base64.b64encode(b'').decode())
    reply.update({'TLE': dict(status='TO'), 'RE': dict(status='RE', exitCode=1), 'MLE': dict(oom=True)}.get(verdict, {}))
    reply.update(changes)
    return reply


class Judge:
    """Runs host.judge with sandbox and interactive programs replaced by scripted replies."""

    def __init__(self, test, submission=None, checker=None, interactor=None, aggregator=None, compiled=None):
        self.test, self.calls, self.executed, self.aggregator_input = test, [], [], None
        self.submission = submission or (lambda index: finished())
        self.checker, self.interactor, self.aggregator = checker, interactor, aggregator
        self.compiled = compiled or {}

    def execute(self, request, compile_phase=False, **kwargs):
        if compile_phase:
            self.calls.append(('compile', request['source']))
            Path(kwargs.get('artifact', sandbox.ARTIFACT)).write_bytes(request['source'].encode())
            return dict(compiled=self.compiled.get(request['source'], True), compileLog='log of ' + request['source'])
        if kwargs.get('stderr_log'):
            self.calls.append(('aggregate',))
            self.test.assertEqual(Path(kwargs['artifact']).read_bytes(), b'aggregator source')
            self.test.assertEqual((request['timeLimitMs'], request['memoryLimitMb']), (5000, 512))
            self.aggregator_input = base64.b64decode(request['input']).decode()
            return self.aggregator(self.aggregator_input)
        if 'checker_files' in kwargs:
            index = int(kwargs['checker_files']['test-input'])
            self.calls.append(('check', index, kwargs.get('score_file')))
            return self.checker(index)
        index = int(base64.b64decode(request['input']))
        self.calls.append(('run', index))
        self.executed.append(index)
        return self.submission(index)

    def interact(self, job, case, artifact, diagnostic, score_file=False):
        index = int(case['input'])
        self.calls.append(('interact', index, score_file))
        self.executed.append(index)
        return self.interactor(index)

    def __call__(self, job, progress=None):
        with tempfile.TemporaryDirectory() as tmp, patch.object(sandbox, 'execute', self.execute), \
                patch.object(interactive, 'execute', self.interact), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), \
                patch.object(sandbox, 'META', Path(tmp) / 'meta'), \
                patch.object(host.telemetry, 'note_failure') as note:
            result = host.judge(job, 'sha256:test', progress=progress)
            self.test.assertEqual(list(Path(tmp).iterdir()), [])
        self.failure = note.call_args.args if note.called else None
        return result


def aggregator_output(text):
    return lambda _: finished(output=base64.b64encode(text.encode()).decode(), checkerLog='')


class ValidationTests(unittest.TestCase):
    def test_valid_scoring_jobs_and_unchanged_binary_jobs(self):
        host.validate_job(scoring_job(), 'sha256:test')
        host.validate_job(scoring_job('aggregate', members=([], None, ['b', 'a'])), 'sha256:test')
        job = scoring_job(groups=[('g%d' % i, 0 if i else 1000000) for i in range(100)], members=[['g99']])
        host.validate_job(job, 'sha256:test')
        # Without scoring, per-case group keys stay unknown keys, as before.
        binary = scoring_job(members=(['nowhere'], 7, {'x': 1}))
        del binary['scoring']
        host.validate_job(binary, 'sha256:test')
        binary['scoring'] = None
        host.validate_job(binary, 'sha256:test')

    def test_rejects_invalid_scoring(self):
        def invalid(**changes):
            job = scoring_job(changes.pop('mode', 'groups'), **{key: changes.pop(key) for key in ('groups', 'members') if key in changes})
            for key, value in changes.items():
                if key.startswith('scoring_'):
                    job['scoring'][key.removeprefix('scoring_')] = value
                else:
                    job[key] = value
            return job
        cases = [
            invalid(scoring=[]), invalid(scoring_mode='binary'), invalid(scoring_mode=None),
            invalid(easyTest=True), invalid(generate=True, generationPrefix='test-files/' + 'a' * 32 + '/11111111-1111-4111-8111-111111111111/generated/'),
            invalid(validate=True), invalid(scoring_groups=None), invalid(groups=()),
            invalid(groups=[('g%d' % i, 1) for i in range(101)], members=([],)),
            invalid(groups=(('a', -1), ('b', 60))), invalid(groups=(('a', 1.0), ('b', 60))),
            invalid(groups=(('a', True), ('b', 60))), invalid(groups=(('a', '40'), ('b', 60))),
            invalid(groups=(('a', 1000000), ('b', 1))), invalid(groups=(('a', 0), ('b', 0))),
            invalid(groups=(('a', 40), ('a', 60)), members=([],)), invalid(groups=(('a' * 65, 40),), members=([],)),
            invalid(groups=((None, 40),), members=([],)), invalid(scoring_groups=[dict(points=40)]),
            invalid(members=(['c'],)), invalid(members=(['a', 'a'],)), invalid(members=('a',)),
            invalid(members=(0,)), invalid(members=([0],)), invalid(members=({'a': 1},)),
            invalid(scoring_aggregator=dict(runtime='cpp17', source='x')),
            invalid(mode='aggregate', scoring_aggregator=None),
            invalid(mode='aggregate', scoring_aggregator=dict(runtime='sh', source='x')),
            invalid(mode='aggregate', scoring_aggregator=dict(runtime='cpp17', source=' ')),
            invalid(mode='aggregate', scoring_aggregator=dict(runtime='cpp17', source='a\0')),
            invalid(mode='aggregate', scoring_aggregator=dict(runtime='cpp17', source='あ' * 22000)),
            invalid(mode='aggregate', scoring_aggregator='cpp17'),
        ]
        sample = scoring_job()
        sample['cases'][0]['isSample'] = 'true'
        cases.append(sample)
        for job in cases:
            with self.subTest(scoring=job.get('scoring'), cases=job['cases']), self.assertRaises(ValueError):
                host.validate_job(job, 'sha256:test')


class CutoffTests(unittest.TestCase):
    # (membership, verdict if the case runs)
    PLAN = [([], 'AC'), (['a'], 'TLE'), (['a', 'b'], 'TLE'), (['a'], 'AC'), (['b', 'a'], 'AC'),
            (['b'], 'TLE'), (['b'], 'AC'), (['a', 'b'], 'AC'), ([], 'TLE'), ([], 'AC'), (['c'], 'AC')]

    def test_tles_cut_groups_and_skip_cases_only_when_all_their_groups_are_cut(self):
        for interactive_mode in (False, True):
            with self.subTest(interactive=interactive_mode):
                job = scoring_job(groups=(('a', 10), ('b', 20), ('c', 30)), members=[m for m, _ in self.PLAN])
                if interactive_mode:
                    job['interactor'] = dict(runtime='cpp17', source='interactor')
                verdicts = [v for _, v in self.PLAN]
                progress = []
                judge = Judge(self, submission=lambda i: finished(verdicts[i]),
                              interactor=lambda i: dict(verdict=verdicts[i], cpuTimeMs=1, wallTimeMs=1, memoryBytes=1))
                result = judge(job, progress=lambda *args: progress.append(args))
                # a is cut after cases 1 and 2, b after cases 2 and 5; ungrouped cases always run.
                self.assertEqual(judge.executed, [0, 1, 2, 4, 5, 8, 9, 10])
                self.assertEqual([c['verdict'] for c in result['cases']],
                                 ['AC', 'TLE', 'TLE', 'SKIPPED', 'AC', 'TLE', 'SKIPPED', 'SKIPPED', 'TLE', 'AC', 'AC'])
                for i in (3, 6, 7):
                    self.assertEqual(result['cases'][i], dict(name=f'c{i}', verdict='SKIPPED'))
                self.assertEqual((result['verdict'], result['passed'], result['total']), ('TLE', 4, 11))
                self.assertEqual(result['scoring'], dict(score=3000, maxScore=6000, groups=[
                    dict(score=0, maxScore=1000), dict(score=0, maxScore=2000), dict(score=3000, maxScore=3000)]))
                self.assertTrue(all('score' not in c for c in result['cases']))
                self.assertEqual([p[1] for p in progress[2:]], list(range(1, 12)))

    def test_groups_score_only_when_non_empty_and_all_ac(self):
        job = scoring_job(groups=(('all', 10), ('empty', 20), ('wrong', 30), ('sample', 40)),
                          members=(['all', 'wrong'], ['all'], ['wrong'], [], ['sample']))
        job['cases'][3]['isSample'] = True
        verdicts = ['AC', 'AC', 'WA', 'WA', 'AC']
        job['checker'] = dict(runtime='cpp17', source='checker')
        judge = Judge(self, checker=lambda i: finished(exitCode=0 if verdicts[i] == 'AC' else 1, status='' if verdicts[i] == 'AC' else 'RE'))
        result = judge(job)
        self.assertEqual(result['verdict'], 'WA')
        # The ungrouped sample WA does not cost points; the empty group never scores.
        self.assertEqual(result['scoring'], dict(score=5000, maxScore=10000, groups=[
            dict(score=1000, maxScore=1000), dict(score=0, maxScore=2000),
            dict(score=0, maxScore=3000), dict(score=4000, maxScore=4000)]))
        # Groups mode never reads the score file.
        self.assertTrue(all(call[2] is False for call in judge.calls if call[0] == 'check'))

    def test_binary_jobs_keep_the_global_cutoff_and_result_shape(self):
        job = scoring_job(members=[m for m, _ in self.PLAN])
        del job['scoring']
        verdicts = [v for _, v in self.PLAN]
        judge = Judge(self, submission=lambda i: finished(verdicts[i]))
        result = judge(job)
        self.assertEqual(judge.executed, [0, 1, 2])
        self.assertEqual(result['cases'][3:], [dict(name=f'c{i}', verdict='SKIPPED') for i in range(3, 11)])
        self.assertNotIn('scoring', result)

    def test_compile_error_scores_zero_only_for_scoring_jobs(self):
        for scoring in (True, False):
            job = scoring_job()
            if not scoring:
                del job['scoring']
            result = Judge(self, compiled={'submitted source': False})(job)
            expected = dict(verdict='CE', passed=0, total=2, compileLog='log of submitted source')
            if scoring:
                expected['scoring'] = dict(score=0, maxScore=10000, groups=[dict(score=0, maxScore=4000), dict(score=0, maxScore=6000)])
            self.assertEqual(result, expected)


class BudgetTests(unittest.TestCase):
    def run_clock(self, job):
        now = [0]
        def submission(index):
            now[0] += 500
            return finished()
        judge = Judge(self, submission=submission)
        with patch.object(host.time, 'monotonic', lambda: now[0]):
            return judge, judge(job)

    def test_scoring_jobs_skip_cases_after_the_budget_instead_of_je(self):
        job = scoring_job(members=(['a'], ['a'], ['b'], ['b'], []))
        judge, result = self.run_clock(job)
        self.assertEqual(judge.executed, [0, 1, 2])
        self.assertEqual([c['verdict'] for c in result['cases']], ['AC', 'AC', 'AC', 'SKIPPED', 'SKIPPED'])
        # The verdict follows the executed cases; the skipped case costs group b its points.
        self.assertEqual((result['verdict'], result['passed']), ('AC', 3))
        self.assertEqual(result['scoring']['score'], 4000)

    def test_binary_jobs_keep_the_deadline(self):
        job = scoring_job(members=([], [], [], [], []))
        del job['scoring']
        with self.assertRaisesRegex(host.telemetry.PlatformError, 'job_deadline_exceeded'):
            self.run_clock(job)


class ScoreFileTests(unittest.TestCase):
    def test_legacy_score_is_a_bounded_decimal_integer(self):
        for data, score in [(b'', None), (b' \t\r\n', None), (b'0', 0), (b' 42\n', 42), (b'00012', 12),
                            (b'1000000000000000000', 10**18)]:
            self.assertEqual(host.legacy_score(data), score, data)
        for data in [None, b'1000000000000000001', b'99999999999999999999', b'-1', b'+1', b'1.0', b'1e3',
                     b'0x10', b'1 2', '１'.encode(), b'\xff', b'1\x00', b'nan']:
            with self.subTest(data=data), self.assertRaises(host.ScoreError):
                host.legacy_score(data)

    def test_judge_file_must_be_a_bounded_regular_file(self):
        with tempfile.TemporaryDirectory() as tmp:
            box = Path(tmp)
            self.assertIsNone(sandbox.read_judge_file(box, 'legacy'))
            (box / 'score').write_bytes(b'7' * sandbox.SCORE_FILE_LIMIT)
            self.assertEqual(len(sandbox.read_judge_file(box, 'legacy')), sandbox.SCORE_FILE_LIMIT)
            (box / 'score').write_bytes(b'7' * (sandbox.SCORE_FILE_LIMIT + 1))
            self.assertIsNone(sandbox.read_judge_file(box, 'legacy'))
            (box / 'result').write_bytes(b'x' * sandbox.RESULT_FILE_LIMIT)
            self.assertEqual(len(sandbox.read_judge_file(box, 'testlib')), sandbox.RESULT_FILE_LIMIT)
            (box / 'score').unlink()
            (box / 'score').symlink_to(box / 'result')
            self.assertIsNone(sandbox.read_judge_file(box, 'legacy'))
            (box / 'score').unlink()
            os.mkfifo(box / 'score')
            self.assertIsNone(sandbox.read_judge_file(box, 'legacy'))
            (box / 'result').unlink()
            (box / 'result').mkdir()
            self.assertIsNone(sandbox.read_judge_file(box, 'testlib'))

    def test_testlib_result_file(self):
        quitp = testlib_result(b'points', b'37 partial &lt;x&gt; &amp;&quot;', b'37.0')
        self.assertEqual(host.testlib_score(quitp, 7, 'AC'), ('AC', 37, '37 partial <x> &"'))
        for points, score in [(b'0.0', 0), (b'1000000.0', 1000000), (b'12', 12), (b'12.00', 12)]:
            self.assertEqual(host.testlib_score(testlib_result(b'points', b'', points), 7, 'AC')[1], score)
        self.assertEqual(host.testlib_score(testlib_result(b'accepted', b'ok'), 0, 'AC'), ('AC', None, 'ok'))
        self.assertEqual(host.testlib_score(testlib_result(b'wrong-answer', b'no'), 1, 'WA'), ('WA', None, 'no'))
        self.assertEqual(host.testlib_score(b'', 0, 'AC'), ('AC', None, ''))
        for data, code in [(None, 0), (testlib_result(b'points', b'', b'37.5'), 7), (testlib_result(b'points', b'', b'1000001.0'), 7),
                           (testlib_result(b'points', b'', b'-1.0'), 7), (testlib_result(b'points'), 7),
                           (testlib_result(b'accepted'), 7), (b'', 7), (testlib_result(b'points', b'', b'3.0'), 0),
                           (testlib_result(b'partially-correct', b'', pctype=b'0'), 0),
                           (testlib_result(b'partially-correct', b'', pctype=b'1'), 1),
                           (b'37 partial', 7), (b'<result outcome = "points" points = "37.0">x</result>', 7),
                           (testlib_result(b'accepted', b'a<b'), 0)]:
            with self.subTest(data=data, code=code), self.assertRaises(host.ScoreError):
                host.testlib_score(data, code, 'AC' if code in (0, 7) else 'WA')

    def test_only_ac_and_wa_cases_are_read(self):
        for verdict in ('TLE', 'RE', 'MLE', 'OLE', 'JE'):
            self.assertEqual(host.case_score('legacy', verdict, b'garbage', 0), (verdict, None, ''))
            self.assertEqual(host.case_score('testlib', verdict, None, 7), (verdict, None, ''))
        self.assertEqual(host.case_score('legacy', 'WA', b'5\n', 1), ('WA', 5, ''))


class AggregatorTests(unittest.TestCase):
    def test_input_lists_groups_and_cases_by_number(self):
        job = scoring_job('aggregate', groups=(('a b', 10), ('c', 0), ('d', 90)), members=([], ['d', 'a b'], ['c'], ['a b']))
        job['cases'][0]['isSample'] = True
        cases = [dict(verdict='AC', score=5), dict(verdict='WA', score=10**18), dict(verdict='TLE'), dict(verdict='SKIPPED')]
        self.assertEqual(host.aggregator_input(job, cases),
                         '4 3\n10\n0\n90\nAC 5 1 0\nWA 1000000000000000000 0 2 0 2\nTLE - 0 1 1\nSKIPPED - 0 1 0\n')

    def test_output_is_one_decimal_per_group_truncated_to_hundredths(self):
        groups = [dict(points=10), dict(points=100), dict(points=0)]
        for output, scores in [(b'10\n37.5\n0\n', [1000, 3750, 0]), (b'9.999 12.349 0.0', [999, 1234, 0]),
                               (b'\n 10.000\t007.50\r\n0.00000\n', [1000, 750, 0]),
                               (b'00000000000000000001 0.019 0', [100, 1, 0])]:
            self.assertEqual(host.aggregator_scores(output, groups), scores, output)
        for output in [b'10 37.5', b'10 37.5 0 0', b'', b'10.001 0 0', b'11 0 0', b'10 100.01 0', b'0 0 0.001',
                       b'-1 0 0', b'+1 0 0', b'1e1 0 0', b'nan 0 0', b'inf 0 0', b'.5 0 0', b'5. 0 0', b'1,5 0 0',
                       '１ 0 0'.encode(), b'99999999999999999999 0 0', b'1 0 0' + b' ' * 65536]:
            with self.subTest(output=output), self.assertRaises(host.ScoreError):
                host.aggregator_scores(output, groups)

    def legacy_job(self, **changes):
        job = scoring_job('aggregate', groups=(('a', 10), ('b', 20)), members=(['a'], ['a', 'b'], ['b'], []))
        job['cases'][3]['isSample'] = True
        job['checker'] = dict(runtime='cpp17', source='checker')
        job.update(changes)
        return job

    def test_legacy_checker_scores_reach_the_aggregator(self):
        verdicts = ['AC', 'WA', 'RE', 'AC']
        files = [b'7\n', b' 3 ', b'ignored', b'']
        judge = Judge(self, submission=lambda i: finished('RE' if verdicts[i] == 'RE' else 'AC'),
                      checker=lambda i: finished(exitCode=1 if verdicts[i] == 'WA' else 0, status='RE' if verdicts[i] == 'WA' else '',
                                                 scoreFile=files[i], checkerLog=f'checker {i}'),
                      aggregator=lambda _: finished(output=base64.b64encode(b'9.999\n12.5\n').decode(), checkerLog='aggregator note'))
        result = judge(self.legacy_job())
        self.assertEqual(judge.aggregator_input, '4 2\n10\n20\nAC 7 0 1 0\nWA 3 0 2 0 1\nRE - 0 1 1\nAC - 1 0\n')
        # The aggregator compiles before any case runs and runs once after all of them.
        self.assertEqual([c[0] for c in judge.calls], ['compile', 'compile', 'compile', 'run', 'check', 'run', 'check',
                                                       'run', 'run', 'check', 'aggregate'])
        self.assertEqual(judge.calls[2], ('compile', 'aggregator source'))
        self.assertTrue(all(call[2] is True for call in judge.calls if call[0] == 'check'))
        self.assertEqual([c.get('score') for c in result['cases']], [7, 3, None, None])
        self.assertEqual((result['verdict'], result['passed']), ('WA', 2))
        self.assertEqual(result['scoring'], dict(score=2249, maxScore=3000, groups=[dict(score=999, maxScore=1000),
                                                                                     dict(score=1250, maxScore=2000)]))
        self.assertIn('ケース1: AC (点: 7)\nchecker 0', result['checkerLog'])
        self.assertIn('集計コードの標準エラー:\naggregator note', result['checkerLog'])

    def test_failures_are_je_with_a_kept_diagnostic(self):
        long_log = 'x' * 20000
        def checker(score):
            return lambda i: finished(scoreFile=score, checkerLog=long_log)
        output = aggregator_output('1 1')
        for name, judge, reason, message in [
                ('score', Judge(self, checker=checker(b'1.5'), aggregator=output), 'judge_score_invalid', '10進の整数ではありません'),
                ('file', Judge(self, checker=checker(None), aggregator=output), 'judge_score_invalid', '通常のファイルでないか'),
                ('compile', Judge(self, checker=checker(b''), aggregator=output, compiled={'aggregator source': False}),
                 'aggregator_compile_failed', '集計コードのコンパイル失敗\nlog of aggregator source'),
                ('runtime', Judge(self, checker=checker(b''), aggregator=lambda _: finished('TLE', checkerLog='slow')),
                 'aggregator_execution_failed', '集計コードの実行失敗: TLE\nslow'),
                ('signal', Judge(self, checker=checker(b''), aggregator=lambda _: finished(status='SG', signal=6, checkerLog='abort')),
                 'aggregator_execution_failed', '集計コードの実行失敗: RE\nabort'),
                ('memory', Judge(self, checker=checker(b''), aggregator=lambda _: finished('MLE', checkerLog='')),
                 'aggregator_execution_failed', '集計コードの実行失敗: MLE'),
                ('output', Judge(self, checker=checker(b''), aggregator=lambda _: finished(output=base64.b64encode(b'1 nan').decode(), checkerLog='e')),
                 'aggregator_output_invalid', 'グループ2の得点「nan」が10進の小数ではありません。\ne'),
                ('range', Judge(self, checker=checker(b''), aggregator=aggregator_output('10.01 1')),
                 'aggregator_output_invalid', 'グループ1の得点が、0以上10以下ではありません。')]:
            with self.subTest(name):
                result = judge(self.legacy_job())
                self.assertEqual(set(result), {'verdict', 'passed', 'total', 'checkerLog'})
                self.assertEqual((result['verdict'], result['passed'], result['total']), ('JE', 0, 4))
                self.assertEqual(judge.failure, ('judge_code', reason))
                self.assertIn(message, result['checkerLog'])
                self.assertLessEqual(len(result['checkerLog'].encode()), 16384)
                if name == 'compile':
                    self.assertEqual(judge.executed, [])

    def test_testlib_checker_points_in_aggregate_mode(self):
        job = scoring_job('aggregate', groups=(('a', 100),), members=(['a'], ['a']))
        job['checker'] = dict(runtime='cpp23-gcc', protocol='testlib', source='checker')
        quitp = testlib_result(b'points', b'37 partial', b'37.0')
        judge = Judge(self, checker=lambda i: finished(exitCode=7, status='RE', scoreFile=quitp, checkerLog='See file to check exit message\n'),
                      aggregator=aggregator_output('74'))
        result = judge(job)
        self.assertEqual(judge.aggregator_input, '2 1\n100\nAC 37 0 1 0\nAC 37 0 1 0\n')
        self.assertEqual([c['score'] for c in result['cases']], [37, 37])
        self.assertEqual(result['scoring']['score'], 7400)
        self.assertIn('結果ファイル: 37 partial', result['checkerLog'])
        self.assertNotIn('未対応', result['checkerLog'])
        for exit_code, data in [(7, testlib_result(b'points', b'', b'0.5')), (1, testlib_result(b'partially-correct', b'', pctype=b'1')),
                                (0, testlib_result(b'partially-correct', b'', pctype=b'0')), (7, b'')]:
            with self.subTest(exit_code=exit_code, data=data):
                judge = Judge(self, checker=lambda i: finished(exitCode=exit_code, status='RE' if exit_code else '', scoreFile=data, checkerLog=''),
                              aggregator=aggregator_output('0'))
                self.assertEqual(judge(job)['verdict'], 'JE')
                self.assertEqual(judge.failure, ('judge_code', 'judge_score_invalid'))
        judge = Judge(self, checker=lambda i: finished(exitCode=1, status='RE', scoreFile=testlib_result(b'wrong-answer', b'no'), checkerLog=''),
                      aggregator=aggregator_output('0'))
        result = judge(job)
        self.assertEqual(result['verdict'], 'WA')
        self.assertEqual(judge.aggregator_input, '2 1\n100\nWA - 0 1 0\nWA - 0 1 0\n')

    def test_testlib_points_stay_je_outside_aggregate_mode(self):
        for scoring, note in [(True, 'testlibの部分点（_points）は、集計コード方式の問題でだけ使えます。'),
                              (False, 'testlibの部分点（_points）は未対応です。')]:
            job = scoring_job()
            if not scoring:
                del job['scoring']
            job['checker'] = dict(runtime='cpp23-gcc', protocol='testlib', source='checker')
            judge = Judge(self, checker=lambda i: finished(exitCode=7, status='RE', checkerLog=''))
            result = judge(job)
            self.assertEqual(result['verdict'], 'JE')
            self.assertIn(note, result['checkerLog'])
            self.assertTrue(all(call[2] is False for call in judge.calls if call[0] == 'check'))

    def test_interactor_scores(self):
        job = scoring_job('aggregate', groups=(('a', 100),), members=(['a'], ['a'], ['a']))
        job['interactor'] = dict(runtime='cpp23-gcc', protocol='testlib', source='interactor')
        replies = [dict(verdict='AC', scoreFile=testlib_result(b'points', b'12 ok', b'12.0'), judgeExitCode=7),
                   dict(verdict='WA', scoreFile=testlib_result(b'wrong-answer', b'no'), judgeExitCode=1),
                   dict(verdict='TLE', scoreFile=b'garbage', judgeExitCode=None)]
        judge = Judge(self, interactor=lambda i: dict(cpuTimeMs=1, wallTimeMs=1, memoryBytes=1, **replies[i]),
                      aggregator=aggregator_output('12'))
        result = judge(job)
        self.assertEqual([call[2] for call in judge.calls if call[0] == 'interact'], [True, True, True])
        self.assertEqual(judge.aggregator_input, '3 1\n100\nAC 12 0 1 0\nWA - 0 1 0\nTLE - 0 1 0\n')
        self.assertEqual(result['cases'][0], dict(name='c0', verdict='AC', score=12, cpuTimeMs=1, wallTimeMs=1, memoryBytes=1))
        self.assertTrue(all('scoreFile' not in c and 'judgeExitCode' not in c for c in result['cases']))
        job['interactor']['protocol'] = 'legacy'
        replies[0].update(scoreFile=b'-5', judgeExitCode=0)
        judge = Judge(self, interactor=lambda i: dict(cpuTimeMs=1, wallTimeMs=1, memoryBytes=1, **replies[i]),
                      aggregator=aggregator_output('12'))
        self.assertEqual(judge(job)['verdict'], 'JE')
        self.assertEqual(judge.failure, ('judge_code', 'judge_score_invalid'))


class SandboxContractTests(unittest.TestCase):
    def test_result_file_arguments_only_in_aggregate_mode(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            artifact = root / 'artifact'
            artifact.write_bytes(b'binary')
            files = {'test-input': b'in', 'expected-output': b'ans', 'submission-source': b'src', 'submission-output': b'out'}
            for interactive_mode in (False, True):
                box = root / str(interactive_mode)
                box.mkdir()
                command = sandbox.prepare_program(box, RUNTIMES['cpp23-gcc-isolate'], artifact, files, protocol='testlib',
                                                  interactive=interactive_mode, result_file=True)
                output = 'test-output' if interactive_mode else 'submission-output'
                self.assertEqual(command[1:], ['/box/test-input', '/box/' + output, '/box/expected-output', '/box/result', '-appes'])
                self.assertEqual((box / 'result').read_bytes(), b'')
                self.assertEqual((box / 'result').stat().st_mode & 0o777, 0o666)
                self.assertFalse((box / 'score').exists())
            box = root / 'legacy'
            box.mkdir()
            command = sandbox.prepare_program(box, RUNTIMES['cpp17-isolate'], artifact, files, result_file=True)
            self.assertEqual(command[1:], ['/box/test-input', '/box/expected-output', '/box/submission-source', '/box/score'])
            self.assertFalse((box / 'result').exists())

    def test_execute_reads_the_judge_file_and_aggregator_stderr(self):
        for protocol, name, kwargs in [('legacy', 'score', dict(score_file=True)), ('testlib', 'result', dict(score_file=True)),
                                       ('legacy', None, dict(stderr_log=True)), ('legacy', None, {})]:
            with self.subTest(protocol=protocol, kwargs=kwargs), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                box = root / 'box'
                box.mkdir()
                artifact, meta = root / 'artifact', root / 'meta'
                artifact.write_bytes(b'binary')
                def invoke(args, timeout=10):
                    if '--run' in args:
                        meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024')
                        (box / 'stdout').write_bytes(b'1.5\n')
                        (box / 'stderr').write_bytes(b'note')
                        if name:
                            (box / name).write_bytes(b'42\n')
                    return subprocess.CompletedProcess(args, 0, stdout=str(root).encode())
                checker_files = None if 'stderr_log' in kwargs or not kwargs else {
                    'test-input': b'in', 'expected-output': b'ans', 'submission-source': b'src'}
                runtime = 'cpp23-gcc-isolate' if protocol == 'testlib' else 'cpp17-isolate'
                with patch.object(sandbox, 'invoke', invoke), patch.object(sandbox, 'META', meta):
                    result = sandbox.execute(dict(runtime=runtime, input='', protocol=protocol, timeLimitMs=5000, memoryLimitMb=512),
                                             artifact=artifact, checker_files=checker_files, **kwargs)
                self.assertEqual(result.get('scoreFile'), b'42\n' if name else None)
                self.assertEqual('scoreFile' in result, bool(name))
                self.assertEqual(result.get('checkerLog'), 'note' if kwargs else None)

    def test_judge_verdict_accepts_points_only_when_asked(self):
        points = dict(status='RE', exitCode=7, signal=0, oom=False, overflow=False)
        self.assertEqual(sandbox.judge_verdict(points, 'testlib'), 'JE')
        self.assertEqual(sandbox.judge_verdict(points, 'testlib', points=True), 'AC')
        self.assertEqual(sandbox.judge_verdict(dict(points, signal=6, status='SG'), 'testlib', points=True), 'JE')
        self.assertEqual(sandbox.judge_verdict(dict(points, oom=True), 'testlib', points=True), 'JE')
        self.assertEqual(sandbox.judge_verdict(points, 'legacy', points=True), 'WA')

    def test_interactive_reads_the_interactor_file_after_both_programs_stop(self):
        from test_checker import job
        for protocol, name in (('legacy', 'score'), ('testlib', 'result')):
            with self.subTest(protocol=protocol), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                for i in range(2):
                    (root / str(i) / 'box').mkdir(parents=True)
                (root / '1' / 'box' / name).write_bytes(b'5')
                request = job()
                request['interactor'] = dict(request.pop('checker'), protocol=protocol, runtime='cpp23-gcc')
                prepared, relayed = [], []
                def invoke(args, *, box_id=0, **kwargs):
                    return subprocess.CompletedProcess(args, 0, stdout=str(root / str(box_id)).encode())
                def popen(command, **kwargs):
                    meta = Path(next(arg.removeprefix('--meta=') for arg in command if arg.startswith('--meta=')))
                    meta.write_text('time:0.1\ntime-wall:0.2\ncg-mem:1024' + ('\nstatus:RE\nexitcode:7' if 'interactive-1' in meta.name else ''))
                    return Mock(returncode=0, poll=Mock(return_value=0))
                def prepare(box, runtime, binary, files, **kwargs):
                    prepared.append(kwargs.get('result_file'))
                    return ['/box/main']
                def relay(*args, **kwargs):
                    relayed.append(kwargs.get('points'))
                    return 'AC'
                with patch.object(interactive.sandbox, 'invoke', invoke), \
                        patch.object(interactive.sandbox, 'META', root / 'meta'), \
                        patch.object(interactive.sandbox, 'prepare_program', prepare), \
                        patch.object(interactive.subprocess, 'Popen', popen), \
                        patch.object(interactive, 'relay', relay):
                    result = interactive.execute(request, request['cases'][0], root / 'checker', lambda _: None, score_file=True)
                self.assertEqual(prepared, [False, True])
                self.assertEqual(relayed, [True])
                self.assertEqual((result['scoreFile'], result['judgeExitCode']), (b'5', 7))


if __name__ == '__main__':
    unittest.main()
