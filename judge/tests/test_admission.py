import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from admission import publication


class AdmissionTests(unittest.TestCase):
    def test_only_passed_runtimes_can_be_published(self):
        report = {'runtimeDigest': 'sha256:' + 'a' * 64,
                  'passedRuntimes': ['cpp23-gcc-isolate', 'c23-gcc-isolate']}
        self.assertEqual(publication(report, 'cpp23-gcc,c23-gcc'),
                         (report['runtimeDigest'], 'cpp23-gcc,c23-gcc'))
        for requested in ('java24', 'cpp23-gcc,java24', '', 'cpp23-gcc,'):
            with self.assertRaises(ValueError):
                publication(report, requested)

    def test_unpinned_report_is_rejected(self):
        for digest in ('latest', '', 'sha256:' + 'x' * 64):
            with self.assertRaises(ValueError):
                publication({'runtimeDigest': digest, 'passedRuntimes': ['cpp23-gcc-isolate']}, 'cpp23-gcc')

    def test_malformed_or_contradictory_report_is_rejected(self):
        for extra in ({'passedRuntimes': 'cpp23-gcc-isolate-unverified'},
                      {'passedRuntimes': ['cpp23-gcc-isolate'], 'failedRuntimes': ['cpp23-gcc-isolate']}):
            with self.assertRaises(ValueError):
                publication({'runtimeDigest': 'sha256:' + 'a' * 64, **extra}, 'cpp23-gcc')
