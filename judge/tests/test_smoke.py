import contextlib
import io
import json
import os
from pathlib import Path
import runpy
import sys
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1] / 'runner'
sys.path.insert(0, str(ROOT))
import host
import interactive_smoke


class SmokeReportTests(unittest.TestCase):
    def run_smoke(self, judge, role_error=None):
        output = io.StringIO()
        fixtures = (ROOT / 'language-smoke.json').read_text()
        with patch.object(host, 'judge', side_effect=judge), \
                patch.object(host, 'slot', return_value=contextlib.nullcontext()), \
                patch.object(host, 'prepare_cgroup'), \
                patch.object(host, 'verify_assets', return_value='sha256:' + 'a' * 64), \
                patch.object(interactive_smoke, 'run', side_effect=role_error), \
                patch.object(Path, 'read_text', return_value=fixtures), \
                patch.dict(os.environ, JUDGE_SMOKE_RUNTIMES='cpp23-gcc-isolate,c23-gcc-isolate'), \
                contextlib.redirect_stdout(output):
            try:
                runpy.run_path(str(ROOT / 'smoke.py'))
            except (AssertionError, SystemExit) as error:
                return output.getvalue(), error
        return output.getvalue(), None

    def test_common_isolation_failure_produces_no_publication_report(self):
        output, error = self.run_smoke(lambda *_: {'verdict': 'JE'})
        self.assertIsInstance(error, AssertionError)
        self.assertNotIn('passedRuntimes', output)

    def test_failed_language_is_excluded_without_skipping_next_language(self):
        baseline = iter(['AC', 'WA', 'CE', 'TLE', 'TLE', 'MLE', 'MLE', 'AC', 'OLE', 'AC', 'AC', 'AC', 'AC', 'AC', 'AC',
                         'AC', 'MLE', 'AC', 'MLE', 'AC', 'MLE'])
        def judge(job, _, save_output=None):
            first = next(baseline, None)
            if first:
                return {'verdict': first}
            if job.get('checker'):
                return {'verdict': 'WA' if 'assert(0)' in job['checker']['source'] or job['checker']['runtime'] != 'c23-gcc' else 'AC'}
            if job['runtime'] == 'cpp23-gcc-isolate':
                return {'verdict': 'WA'}
            if job.get('generate'):
                for _ in job['cases']:
                    save_output(b'3\n')
                return {'verdict': 'AC'}
            verdict = 'CE' if job['source'] == 'not c' else 'TLE' if 'for(;;)' in job['source'] else 'WA' if job['cases'][0]['output'] == '4' else 'AC'
            return {'verdict': verdict}
        output, error = self.run_smoke(judge)
        self.assertIsInstance(error, SystemExit)
        report = json.loads(output.splitlines()[-1])
        self.assertEqual(report['passedRuntimes'], ['c23-gcc-isolate'])
        self.assertEqual(report['failedRuntimes'], ['cpp23-gcc-isolate'])
        baseline = iter(['AC', 'WA', 'CE', 'TLE', 'TLE', 'MLE', 'MLE', 'AC', 'OLE', 'AC', 'AC', 'AC', 'AC', 'AC', 'AC',
                         'AC', 'MLE', 'AC', 'MLE', 'AC', 'MLE'])
        output, error = self.run_smoke(judge, AssertionError('interactor exceeded 256 MiB'))
        self.assertIsInstance(error, SystemExit)
        report = json.loads(output.splitlines()[-1])
        self.assertEqual(report['passedRuntimes'], [])
        self.assertEqual(set(report['failedRuntimes']), {'cpp23-gcc-isolate', 'c23-gcc-isolate'})


class RunTwiceSmokeTests(unittest.TestCase):
    PASSED = ['python314-isolate', 'cpp23-gcc-isolate']

    def results(self):
        skipped = '1回目:\n2回目: 実行しません（1回目の判定: WA）\n'
        return [
            {'verdict': 'AC', 'checkerLog': '1回目:\n2回目:\n'},
            {'verdict': 'WA', 'checkerLog': skipped},
            {'verdict': 'WA', 'checkerLog': '1回目:\n2回目:\n'},
            {'verdict': 'AC', 'checkerLog': '1回目:\n2回目:\n'},
            {'verdict': 'AC', 'cases': [{'score': 6}], 'scoring': dict(score=600, maxScore=1000, groups=[dict(score=600, maxScore=1000)])},
            {'verdict': 'AC', 'checkerLog': '1回目:\n2回目:\n'},
            {'verdict': 'AC', 'cases': [{'score': 37}], 'scoring': dict(score=3700, maxScore=10000, groups=[dict(score=3700, maxScore=10000)])},
        ]

    def run_with(self, results, passed=None):
        jobs, queue = [], iter(results)
        def judge(job, runtime, **_):
            jobs.append(job)
            return next(queue)
        interactive_smoke.run_twice('sha256:' + 'a' * 64, self.PASSED if passed is None else passed, {}, judge)
        return jobs

    def test_run_twice_smoke_covers_both_protocols(self):
        jobs = self.run_with(self.results())
        self.assertEqual(len(jobs), 7)
        for job in jobs:
            self.assertIn('runTwice', job)
            self.assertNotIn('interactor', job)
            self.assertNotIn('checker', job)
            self.assertEqual(job['runtime'], 'python314-isolate')
        legacy, first_bad, second_bad, carry, legacy_score, testlib, testlib_score = jobs
        # Legacy: the handoff carries the submission's run-1 reply into run 2.
        self.assertEqual(legacy['runTwice']['runtime'], 'python314')
        self.assertNotIn('protocol', legacy['runTwice'])
        self.assertIn('sys.argv[6]', legacy['runTwice']['source'])
        self.assertIn('sys.argv[5]', legacy['runTwice']['source'])
        self.assertNotEqual(first_bad['source'], legacy['source'])
        self.assertNotEqual(second_bad['source'], legacy['source'])
        # A file the submission wrote in run 1 must not reach run 2.
        self.assertIn("open('carry', 'w')", carry['source'])
        # Aggregate legacy: run 1 writes a decoy score, so only run 2 may set the case score.
        self.assertEqual(legacy_score['scoring']['mode'], 'aggregate')
        self.assertIn("'99'", legacy_score['runTwice']['source'])
        self.assertIn('AC 6 0 1 0', legacy_score['scoring']['aggregator']['source'])
        for job in (testlib, testlib_score):
            self.assertEqual(job['runTwice']['runtime'], 'cpp23-gcc')
            self.assertEqual(job['runTwice']['protocol'], 'testlib')
            self.assertIn('registerInteraction', job['runTwice']['source'])
            self.assertTrue(job['cases'][0]['input'].startswith('0'))
        self.assertNotIn('quitp', testlib['runTwice']['source'])
        self.assertEqual(testlib_score['scoring']['mode'], 'aggregate')
        self.assertIn('quitp(11', testlib_score['runTwice']['source'])
        self.assertIn('quitp(37', testlib_score['runTwice']['source'])
        self.assertIn('AC 37 0 1 0', testlib_score['scoring']['aggregator']['source'])

    def test_wrong_verdicts_stop_the_smoke(self):
        for index, change in [(0, {'verdict': 'WA'}), (1, {'verdict': 'AC'}), (1, {'checkerLog': '1回目:\n'}),
                              (2, {'checkerLog': '2回目: 実行しません（1回目の判定: WA）\n'}),
                              (3, {'verdict': 'WA'}), (4, {'cases': [{'score': 99}]}),
                              (5, {'verdict': 'JE'}), (6, {'scoring': dict(score=1100, maxScore=10000, groups=[])})]:
            results = self.results()
            results[index] = dict(results[index], **change)
            with self.assertRaises(AssertionError, msg=(index, change)):
                self.run_with(results)

    def test_missing_runtimes_skip_dependent_checks(self):
        self.assertEqual(len(self.run_with(self.results()[:4], ['python314-isolate'])), 4)
        self.assertEqual(self.run_with([], ['cpp23-gcc-isolate']), [])
