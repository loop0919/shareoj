"""Burst hosts stop themselves once idle past the holds set by the bridge and operators."""
import datetime
import os
import subprocess
import time
import urllib.error
import urllib.request

import telemetry

# The bridge extends JudgeHoldUntil for contests; rollout owns the maintenance hold.
HOLD_TAGS = ('JudgeHoldUntil', 'JudgeMaintenanceHoldUntil')
IDLE_SECONDS = 600
# A hold extended just before it lapsed may take a moment to reach instance metadata.
GRACE_SECONDS = 60
CHECK_SECONDS = 60
FAILURE_REPORT_SECONDS = 3600


def metadata_endpoint():
    configured = os.environ.get('AWS_EC2_METADATA_SERVICE_ENDPOINT')
    if configured:
        return configured.rstrip('/')
    ipv6 = os.environ.get('AWS_EC2_METADATA_SERVICE_ENDPOINT_MODE', '').lower() == 'ipv6'
    return 'http://[fd00:ec2::254]' if ipv6 else 'http://169.254.169.254'


def instance_tags(endpoint, keys, timeout=2):
    """Read instance tags over IMDSv2; a missing tag is None, any other failure raises."""
    opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
    token_request = urllib.request.Request(endpoint + '/latest/api/token', method='PUT',
                                           headers={'X-aws-ec2-metadata-token-ttl-seconds': '60'})
    with opener.open(token_request, timeout=timeout) as response:
        token = response.read().decode()
    values = {}
    for key in keys:
        request = urllib.request.Request(endpoint + '/latest/meta-data/tags/instance/' + key,
                                         headers={'X-aws-ec2-metadata-token': token})
        try:
            with opener.open(request, timeout=timeout) as response:
                values[key] = response.read().decode()
        except urllib.error.HTTPError as error:
            if error.code != 404:
                raise
            values[key] = None
    return values


def parse_hold(value):
    if value is None:
        return 0.0
    moment = datetime.datetime.fromisoformat(value.replace('Z', '+00:00'))
    if moment.tzinfo is None:
        raise ValueError('hold without a time zone')
    return moment.timestamp()


class IdleStop:
    def __init__(self, read_tags, monotonic=time.monotonic, wall=time.time):
        self.read_tags = read_tags
        self.monotonic = monotonic
        self.wall = wall
        self.idle_since = monotonic()
        self.next_check = 0.0
        self.next_failure_report = 0.0

    @classmethod
    def from_environment(cls):
        role = os.environ.get('JUDGE_POOL_ROLE', '')
        if role in ('', 'primary'):
            return None
        if role != 'burst':
            raise ValueError('unknown pool role')
        endpoint = metadata_endpoint()
        return cls(lambda: instance_tags(endpoint, HOLD_TAGS))

    def touch(self):
        self.idle_since = self.monotonic()

    def due(self):
        now = self.monotonic()
        if now - self.idle_since < IDLE_SECONDS or now < self.next_check:
            return False
        self.next_check = now + CHECK_SECONDS
        try:
            hold = max(parse_hold(value) for value in self.read_tags().values())
        except Exception:
            # Unknown holds keep the host running; a stuck host costs less than a missed contest.
            if now >= self.next_failure_report:
                self.next_failure_report = now + FAILURE_REPORT_SECONDS
                telemetry.emit('failure', category='platform', reason='pool_hold_unavailable')
            return False
        return self.wall() >= hold + GRACE_SECONDS

    def stop(self):
        telemetry.emit('idle_stop')
        # Let the log agent ship the final events before the host goes away.
        time.sleep(10)
        subprocess.run(['systemctl', 'poweroff', '--no-block'], check=True)
