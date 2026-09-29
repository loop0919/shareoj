#!/usr/bin/python3
"""SQS/S3 transport; runs as a single systemd-managed slot on the Lightsail host."""
import base64
import uuid
import hashlib
import json
import telemetry
import os
import signal
import time

from host import judge, prepare_cgroup, verify_assets, pointer, slot


def read_test_file(s3, bucket, item):
    response = s3.get_object(Bucket=bucket, Key=item['key'], VersionId=item['versionId'])
    with response['Body'] as stream:
        data = stream.read(item['size'] + 1)
    if len(data) != item['size'] or hashlib.sha256(data).hexdigest() != item['sha256'] or b'\0' in data:
        raise ValueError('test file integrity')
    try:
        return data.decode('utf-8')
    except UnicodeDecodeError as error:
        raise ValueError('test file encoding') from error


def write_generated_file(s3, bucket, prefix, data):
    if not bucket:
        raise ValueError('output storage unavailable')
    file_id = str(uuid.uuid4())
    digest = hashlib.sha256(data).digest()
    key = prefix + file_id
    response = s3.put_object(Bucket=bucket, Key=key, Body=data, ContentType='text/plain; charset=utf-8',
                             Tagging='status=pending', ChecksumSHA256=base64.b64encode(digest).decode())
    version = response.get('VersionId')
    if not isinstance(version, str) or not version:
        raise ValueError('versioned output storage required')
    return dict(id=file_id, size=len(data), sha256=digest.hex(), key=key, versionId=version)


def progress_reporter(client, queue, item):
    last_phase, last_verdict, last_sent, enabled = None, None, 0, True

    def report(phase, completed, total, verdict=None):
        nonlocal last_phase, last_verdict, last_sent, enabled
        now = time.monotonic()
        # Stage changes and the first failure bypass the one-second progress throttle.
        if not enabled or (phase == last_phase and verdict == last_verdict and now - last_sent < 1):
            return
        if phase != last_phase:
            telemetry.emit('judge_phase', phase=phase)
        last_phase, last_verdict, last_sent = phase, verdict, now
        payload = dict(submissionId=item['submissionId'], attemptId=item['attemptId'],
                       progress=dict(phase=phase, completed=completed, total=total))
        if verdict is not None:
            payload['progress']['verdict'] = verdict
        try:
            client.send_message(QueueUrl=queue, MessageBody=json.dumps(payload))
        except Exception:
            # Optional telemetry must not turn a correct submission into JE or delay every case.
            enabled = False
            telemetry.emit('failure', category='platform', reason='progress_send_failed')
    return report


def release_message(client, queue, message):
    # A stopped worker returns its job now instead of after the 35-minute visibility timeout.
    try:
        client.change_message_visibility(QueueUrl=queue, ReceiptHandle=message['ReceiptHandle'], VisibilityTimeout=0)
    except Exception:
        telemetry.emit('failure', category='platform', reason='request_release_failed')


def process_message(message, sqs, s3, progress_client, runtime, cgroup, bucket, test_bucket, requests, results):
    with telemetry.operation('invalid_queue_envelope'):
        item = pointer(message['Body'])
    with telemetry.submission(item):
        settled = False
        try:
            telemetry.emit('job_received', phase='PREPARING')
            with telemetry.operation('job_fetch_failed'):
                response = s3.get_object(Bucket=bucket, Key=item['key'], VersionId=item['versionId'])
                with response['Body'] as stream:
                    data = stream.read(2 * 1024 * 1024 + 1)
                if len(data) > 2 * 1024 * 1024 or hashlib.sha256(data).hexdigest() != item['sha256']:
                    raise ValueError('job integrity')
                job = json.loads(data)
                if any(job[name] != item[name] for name in ('submissionId', 'attemptId')):
                    raise ValueError('job identity')
            def load_file(entry):
                with telemetry.operation('test_file_fetch_failed'):
                    return read_test_file(s3, test_bucket, entry)
            def save_file(data):
                with telemetry.operation('generated_file_write_failed'):
                    return write_generated_file(s3, test_bucket, job['generationPrefix'], data)
            telemetry.emit('judge_started', phase='PREPARING')
            try:
                result = judge(job, runtime, load_file, progress_reporter(progress_client, results, item), save_file)
            except Exception as error:
                # Cleanup errors override any earlier checker failure.
                for child in cgroup.iterdir():
                    if child.is_dir() and child.name != 'controller' and (child / 'cgroup.procs').read_text().strip():
                        raise telemetry.FatalPlatformError('isolate_cleanup_failed')
                telemetry.failure(error, verdict='JE')
                result = {'verdict': 'JE', 'passed': 0, 'total': 0}
            else:
                if result['verdict'] == 'JE':
                    telemetry.failure(verdict='JE')
            telemetry.emit('judge_finished', phase='JUDGING', verdict=result['verdict'])
            payload = {'submissionId': item['submissionId'], 'attemptId': item['attemptId'], 'result': result}
            with telemetry.operation('result_send_failed'):
                sqs.send_message(QueueUrl=results, MessageBody=json.dumps(payload, allow_nan=False, ensure_ascii=False))
            settled = True
            telemetry.emit('result_sent', phase='DELIVERY', verdict=result['verdict'])
            with telemetry.operation('request_delete_failed'):
                sqs.delete_message(QueueUrl=requests, ReceiptHandle=message['ReceiptHandle'])
            telemetry.emit('completed', phase='DONE', verdict=result['verdict'])
        except (Exception, telemetry.FatalPlatformError) as error:
            telemetry.failure(error)
            if isinstance(error, telemetry.FatalPlatformError):
                error.logged = True
                raise
            # Retry the queue message, without losing its correlation IDs in the log.
            time.sleep(5)
        except SystemExit:
            # SIGTERM; a result already sent is final, so redelivery would only repeat it.
            if not settled:
                release_message(progress_client, requests, message)
            raise


def main():
    import boto3
    from botocore.config import Config

    with telemetry.operation('runtime_verification_failed'):
        runtime = verify_assets(full=True)
        if runtime != os.environ['JUDGE_RUNTIME_DIGEST']:
            raise ValueError('configured runtime does not match assets')
    with telemetry.operation('cgroup_setup_failed'):
        cgroup = prepare_cgroup()
    config = Config(connect_timeout=5, read_timeout=30, retries={'max_attempts': 3}, use_dualstack_endpoint=True)
    sqs = boto3.client('sqs', config=config)
    progress_client = boto3.client('sqs', config=Config(connect_timeout=1, read_timeout=1,
                                   retries={'total_max_attempts': 1}, use_dualstack_endpoint=True))
    s3 = boto3.client('s3', config=config)
    requests = os.environ['JUDGE_REQUEST_QUEUE_URL']
    results = os.environ['JUDGE_RESULT_QUEUE_URL']
    bucket = os.environ['JUDGE_JOB_BUCKET']
    test_bucket = os.environ.get('JUDGE_TEST_DATA_BUCKET', '')
    telemetry.emit('worker_started')
    while True:
        try:
            # A changed runtime must stop, rather than retry forever while looking healthy.
            try:
                if verify_assets() != runtime:
                    raise ValueError('runtime changed')
            except Exception:
                raise telemetry.FatalPlatformError('runtime_verification_failed') from None
            with telemetry.operation('queue_receive_failed'):
                messages = sqs.receive_message(QueueUrl=requests, MaxNumberOfMessages=1, WaitTimeSeconds=20,
                                               VisibilityTimeout=2100).get('Messages', [])
            for message in messages:
                process_message(message, sqs, s3, progress_client, runtime, cgroup,
                                bucket, test_bucket, requests, results)
        except Exception as error:
            telemetry.failure(error)
            time.sleep(5)


if __name__ == '__main__':
    # SIGTERM exits through Python finally blocks, then systemd kills any remaining children.
    def stop(signum, frame):
        raise SystemExit(0)
    signal.signal(signal.SIGTERM, stop)
    telemetry.configure()
    try:
        with slot():
            main()
    except (Exception, telemetry.FatalPlatformError) as error:
        if not getattr(error, 'logged', False):
            telemetry.failure(error)
        raise SystemExit(1) from None
