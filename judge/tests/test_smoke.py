import contextlib
import io
import json
import os
from pathlib import Path
import runpy
import sys
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
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
                patch.dict(os.environ, JUDGE_SMOKE_RUNTIMES='cpp17-isolate,c23-gcc-isolate'), \
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
        baseline = iter(['AC', 'WA', 'CE', 'TLE', 'TLE', 'MLE', 'MLE', 'AC', 'OLE', 'AC', 'AC', 'AC', 'AC',
                         'AC', 'MLE', 'AC', 'MLE', 'AC', 'MLE'])
        def judge(job, _, save_output=None):
            first = next(baseline, None)
            if first:
                return {'verdict': first}
            if job.get('checker'):
                return {'verdict': 'WA' if 'assert(0)' in job['checker']['source'] or job['checker']['runtime'] != 'c23-gcc' else 'AC'}
            if job['runtime'] == 'cpp17-isolate':
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
        self.assertEqual(report['failedRuntimes'], ['cpp17-isolate'])
        baseline = iter(['AC', 'WA', 'CE', 'TLE', 'TLE', 'MLE', 'MLE', 'AC', 'OLE', 'AC', 'AC', 'AC', 'AC',
                         'AC', 'MLE', 'AC', 'MLE', 'AC', 'MLE'])
        output, error = self.run_smoke(judge, AssertionError('interactor exceeded 256 MiB'))
        self.assertIsInstance(error, SystemExit)
        report = json.loads(output.splitlines()[-1])
        self.assertEqual(report['passedRuntimes'], [])
        self.assertEqual(set(report['failedRuntimes']), {'cpp17-isolate', 'c23-gcc-isolate'})
