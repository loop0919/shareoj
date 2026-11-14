from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import verify

DIGEST = 'sha256:' + 'a' * 64
URL = 'https://sqs.ap-northeast-1.amazonaws.com/123456789012/judge-requests-a'


class StartCommandTests(unittest.TestCase):
    def test_start_command_rewrites_the_request_queue(self):
        script = verify.start_command('r', DIGEST, URL)
        self.assertIn('JUDGE_REQUEST_QUEUE_URL=', script)
        self.assertIn(URL, script)
        self.assertIn('expected exactly one request queue setting', script)
        plain = verify.start_command('r', DIGEST)
        self.assertNotIn('JUDGE_REQUEST_QUEUE_URL=', plain)
        self.assertNotIn('request queue', plain)

    def test_start_command_rejects_other_urls(self):
        for value in ['https://example.com/x', URL + '; reboot', URL + '\n', "'$(id)'",
                      'http://sqs.ap-northeast-1.amazonaws.com/123456789012/q', '']:
            with self.subTest(value=value), self.assertRaises(ValueError):
                verify.start_command('r', DIGEST, value)


if __name__ == '__main__':
    unittest.main()
