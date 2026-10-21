"""Bounded, payload-free operational events; never part of a judge result."""
from contextlib import contextmanager
from contextvars import ContextVar
from datetime import datetime, timezone
import json
import logging
from logging.handlers import RotatingFileHandler
import time

context = ContextVar('judge_telemetry', default=None)
logger = logging.getLogger('judge')


class PlatformError(RuntimeError):
    pass


class FatalPlatformError(SystemExit):
    pass


@contextmanager
def operation(reason):
    try:
        yield
    except (PlatformError, FatalPlatformError):
        raise
    except Exception as error:
        raise PlatformError(reason) from error


@contextmanager
def submission(item):
    token = context.set(dict(submissionId=item['submissionId'], attemptId=item['attemptId'],
                             started=time.monotonic(), failure=None))
    try:
        yield
    finally:
        context.reset(token)


def note_failure(category, reason):
    current = context.get()
    if current is not None:
        current['failure'] = (category, reason)


def failure(error=None, verdict=None):
    if isinstance(error, (PlatformError, FatalPlatformError)):
        category, reason = 'platform', str(error)
    elif error is not None:
        category, reason = 'unknown', 'unclassified_exception'
    else:
        category, reason = (context.get() or {}).get('failure') or ('unknown', 'unclassified_je')
    emit('failure', category=category, reason=reason, **({'verdict': verdict} if verdict else {}))


def emit(event, **fields):
    current = context.get() or {}
    record = dict(timestamp=datetime.now(timezone.utc).isoformat(), service='judge-worker', event=event, **fields)
    for key in ('submissionId', 'attemptId'):
        if key in current:
            record[key] = current[key]
    if 'started' in current:
        record['durationMs'] = round((time.monotonic() - current['started']) * 1000)
    try:
        logger.info(json.dumps(record, ensure_ascii=False, allow_nan=False))
    except Exception:
        # A full disk or broken logging handler cannot affect a verdict.
        pass


def configure(path='/var/log/judge/worker.jsonl'):
    logging.raiseExceptions = False
    logger.setLevel(logging.INFO)
    logger.propagate = False
    logger.handlers.clear()
    logger.addHandler(logging.StreamHandler())
    try:
        logger.addHandler(RotatingFileHandler(path, maxBytes=5 * 1024 * 1024, backupCount=2, encoding='utf-8'))
    except OSError:
        emit('failure', category='platform', reason='log_file_unavailable')
