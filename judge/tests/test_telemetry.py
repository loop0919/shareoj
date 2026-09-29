import base64
import hashlib
import io
import json
import logging
import os
import sys
from pathlib import Path
import tempfile
import unittest
from unittest.mock import Mock, patch

from test_runner import host, sandbox, worker
import test_checker
from test_checker import reply
import telemetry
import notify


class TelemetryTests(unittest.TestCase):
    def setUp(self):
        self.output = io.StringIO()
        self.handler = logging.StreamHandler(self.output)
        self.old_handlers = telemetry.logger.handlers[:]
        self.old_level = telemetry.logger.level
        telemetry.logger.handlers = [self.handler]
        telemetry.logger.setLevel(logging.INFO)

    def tearDown(self):
        telemetry.logger.handlers = self.old_handlers
        telemetry.logger.setLevel(self.old_level)

    def events(self):
        return [json.loads(line) for line in self.output.getvalue().splitlines()]

    def test_checker_failures_and_normal_verdicts(self):
        runner = test_checker.CheckerTests()
        cases = [(dict(compiled=False), 'JE'), (dict(checked=reply(oom=True)), 'JE'),
                 (dict(checked=reply(status='TO')), 'JE'),
                 (dict(checked=reply(exitCode=3), checker_runtime='cpp23-gcc', protocol='testlib'), 'JE'),
                 (dict(checked=reply(exitCode=1, status='RE')), 'WA'),
                 (dict(submitted=reply(status='TO')), 'TLE'), (dict(submitted=reply(oom=True)), 'MLE')]
        for options, verdict in cases:
            with telemetry.submission(dict(submissionId='id', attemptId='attempt')):
                result, _ = runner.run_job(**dict({'checked': reply()}, **options))
                self.assertEqual(result['verdict'], verdict)
                failure = telemetry.context.get()['failure']
                self.assertEqual(failure[0] if failure else None, 'judge_code' if verdict == 'JE' else None)
                self.assertNotIn('category', result)

    def test_interactor_je_is_classified_without_changing_result(self):
        job = test_checker.job()
        job['interactor'] = job.pop('checker')
        with tempfile.TemporaryDirectory() as tmp, telemetry.submission(dict(submissionId='id', attemptId='attempt')), \
                patch.object(sandbox, 'ARTIFACT', Path(tmp) / 'main'), patch.object(sandbox, 'META', Path(tmp) / 'meta'), \
                patch.object(sandbox, 'execute', return_value={'compiled': True}), \
                patch.object(host.interactive, 'execute', return_value={'verdict': 'JE'}):
            result = host.judge(job, 'sha256:test')
            self.assertEqual(result['verdict'], 'JE')
            self.assertEqual(telemetry.context.get()['failure'], ('judge_code', 'interactor_execution_failed'))

    def test_isolate_init_failure_is_platform(self):
        with patch.object(sandbox, 'invoke', return_value=Mock(returncode=2)):
            with self.assertRaisesRegex(telemetry.PlatformError, 'isolate_init_failed'):
                sandbox.execute({'runtime': 'cpp17-isolate', 'source': 'test'}, True)

    def process(self, judge_result=None, judge_error=None, s3_error=None, progress=None):
        data = json.dumps(dict(submissionId='id', attemptId='attempt')).encode()
        item = dict(submissionId='id', attemptId='attempt', key='key', versionId='v', sha256=hashlib.sha256(data).hexdigest())
        s3, sqs = Mock(), Mock()
        s3.get_object.return_value = {'Body': io.BytesIO(data)}
        if s3_error:
            s3.get_object.side_effect = s3_error
        with tempfile.TemporaryDirectory() as tmp, patch.object(worker, 'pointer', return_value=item), \
                patch.object(worker, 'judge', return_value=judge_result or dict(verdict='AC'), side_effect=judge_error), \
                patch.object(worker.time, 'sleep'):
            worker.process_message({'Body': 'secret source', 'ReceiptHandle': 'receipt'}, sqs, s3, progress or Mock(), 'runtime', Path(tmp), 'bucket', 'tests', 'requests', 'results')
        return sqs

    def test_stop_during_judging_releases_the_request(self):
        progress = Mock()
        with self.assertRaises(SystemExit):
            self.process(judge_error=SystemExit(0), progress=progress)
        progress.change_message_visibility.assert_called_once_with(QueueUrl='requests', ReceiptHandle='receipt', VisibilityTimeout=0)
        self.assertFalse(any(e['event'] == 'failure' for e in self.events()))

    def test_stop_after_result_keeps_the_request_hidden(self):
        progress = Mock()
        with patch.object(worker.telemetry, 'emit', side_effect=lambda event, **fields: event == 'result_sent' and sys.exit(0)):
            with self.assertRaises(SystemExit):
                self.process(progress=progress)
        progress.change_message_visibility.assert_not_called()

    def test_failed_release_is_reported(self):
        progress = Mock()
        progress.change_message_visibility.side_effect = RuntimeError('network')
        with self.assertRaises(SystemExit):
            self.process(judge_error=SystemExit(0), progress=progress)
        self.assertEqual(self.events()[-1]['reason'], 'request_release_failed')

    def test_fatal_error_is_not_released(self):
        progress = Mock()
        with self.assertRaises(telemetry.FatalPlatformError):
            self.process(judge_error=telemetry.FatalPlatformError('isolate_cleanup_failed'), progress=progress)
        progress.change_message_visibility.assert_not_called()

    def test_transport_and_unknown_failures_do_not_leak_payloads(self):
        self.process(s3_error=RuntimeError('secret credential'))
        self.assertEqual(self.events()[-1]['category'], 'platform')
        self.assertEqual(self.events()[-1]['reason'], 'job_fetch_failed')
        self.assertEqual(self.events()[-1]['attemptId'], 'attempt')
        self.process(judge_error=ValueError('secret expected output'))
        failures = [e for e in self.events() if e['event'] == 'failure']
        self.assertEqual(failures[-1]['category'], 'unknown')
        self.assertNotIn('secret', self.output.getvalue())
        self.assertIsNone(telemetry.context.get())

    def test_cleanup_failure_overrides_checker_and_no_result_is_sent(self):
        with self.assertRaises(telemetry.FatalPlatformError):
            self.process(judge_error=telemetry.FatalPlatformError('isolate_cleanup_failed'))
        self.assertEqual(self.events()[-1]['category'], 'platform')
        self.assertEqual(self.events()[-1]['reason'], 'isolate_cleanup_failed')

    def test_logging_failure_does_not_change_success(self):
        with patch.object(telemetry.logger, 'info', side_effect=OSError('full')):
            sqs = self.process()
        sqs.delete_message.assert_called_once()
        payload = json.loads(sqs.send_message.call_args.kwargs['MessageBody'])
        self.assertEqual(payload['result']['verdict'], 'AC')

    def test_rotation_is_bounded(self):
        with tempfile.TemporaryDirectory() as tmp:
            telemetry.configure(str(Path(tmp) / 'worker.jsonl'))
            file_handler = telemetry.logger.handlers[-1]
            self.assertEqual(file_handler.maxBytes, 5 * 1024 * 1024)
            self.assertEqual(file_handler.backupCount, 2)
            file_handler.close()


class NotificationTests(unittest.TestCase):
    def test_shared_json_secret_reads_only_configured_key(self):
        sdk = Mock()
        sdk.client.return_value.get_secret_value.return_value = {'SecretString': json.dumps({'ALART_DISCORD_WEBHOOK': 'https://discord.com/api/webhooks/test', 'unrelated': 'private'})}
        alarm = dict(AlarmName='judge-dev-judge-platform', NewStateValue='ALARM', StateChangeTime='now')
        with patch.dict(sys.modules, boto3=sdk), patch.dict(os.environ, WEBHOOK_SECRET_ARN='arn:test', WEBHOOK_SECRET_KEY='ALART_DISCORD_WEBHOOK', AWS_REGION='ap-northeast-1', ENVIRONMENT='dev', LOGS_URL='https://example.com'), patch.object(notify, 'send') as send:
            notify.handler({'Records': [{'Sns': {'Message': json.dumps(alarm)}}]}, None)
            self.assertEqual(send.call_args.args[0], 'https://discord.com/api/webhooks/test')
            self.assertNotIn('private', str(send.call_args))

    def test_author_recovery_is_not_claimed_as_fixed_and_mentions_disabled(self):
        with patch.dict(os.environ, AWS_REGION='ap-northeast-1', ENVIRONMENT='dev', LOGS_URL='https://example.com'):
            payload = notify.notification(dict(AlarmName='judge-dev-judge-judge-code', NewStateValue='OK', StateChangeTime='now', NewStateReason='@everyone'))
        self.assertIn('直近の検出なし', payload['content'])
        self.assertNotIn('復旧', payload['content'])
        self.assertEqual(payload['allowed_mentions'], {'parse': []})

    def test_network_errors_never_expose_webhook(self):
        with patch.object(notify, 'build_opener') as build:
            build.return_value.open.side_effect = OSError('https://discord.com/api/webhooks/SECRET')
            with self.assertRaisesRegex(RuntimeError, '^Discord delivery failed$'):
                notify.send('https://discord.com/api/webhooks/SECRET', {})

    def test_rate_limit_is_retried(self):
        from urllib.error import HTTPError
        response = Mock()
        response.__enter__ = Mock(return_value=Mock(status=204))
        response.__exit__ = Mock(return_value=False)
        limited = HTTPError('secret', 429, 'limited', {}, io.BytesIO(b'{"retry_after":0.1}'))
        with patch.object(notify, 'build_opener') as build, patch.object(notify.time, 'sleep') as sleep:
            build.return_value.open.side_effect = [limited, response]
            notify.send('https://discord.com/api/webhooks/SECRET', {})
            self.assertEqual(build.return_value.open.call_count, 2)
            sleep.assert_called_once_with(.1)
