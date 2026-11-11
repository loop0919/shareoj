"""Compare output with the expected output within an error tolerance (誤差ジャッジ).

Tokens are split as in plain judging. Token counts must agree before any number is compared.
A pair matches when its bytes are equal, or when both are numbers whose exact difference is
within the threshold. This runs in the manager on untrusted output, so it reads one chunk at a
time and every number is bounded by MAX_TOKEN and MAX_EXPONENT.
"""
import decimal
from decimal import Decimal
import re

EPSILONS = tuple(f'1e-{k}' for k in range(1, 13))
DEFAULT_MARGIN = '0.1'
MODES = ('absolute', 'relative', 'either')
MAX_TOKEN = 400
MAX_EXPONENT = 400

_MARGIN = re.compile(r'[0-9]{1,3}(?:\.[0-9]{1,6})?')
# Each run of digits ends at a '.' or 'e' that it cannot match, so a failed match backtracks linearly.
_NUMBER = re.compile(rb'-?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE]([+-]?[0-9]+))?')
# The separators of plain judging; bytes.split() splits on exactly these six bytes.
_SPACE = re.compile(rb'[ \t\n\r\v\f]')
_SPACES = frozenset(b' \t\n\r\v\f')
# Maps a separator to b' ' and any other byte to b'x', so b' x' marks the start of a token.
_CLASSES = bytes(0x20 if byte in _SPACES else 0x78 for byte in range(256))
_CHUNK = 16 * 1024
# A number is c * 10**q with at most MAX_TOKEN digits in c, q >= -(MAX_TOKEN + MAX_EXPONENT) and
# |value| < 10**(MAX_TOKEN + MAX_EXPONENT), so the difference of two numbers has at most
# 2 * (MAX_TOKEN + MAX_EXPONENT) + 1 digits and the threshold (9 digits) times a number at most
# MAX_TOKEN + 9. Nothing rounds at this precision; the traps make any rounding an error instead
# of a verdict.
_PRECISION = 2 * (MAX_TOKEN + MAX_EXPONENT) + 10


def _context():
    return decimal.Context(prec=_PRECISION, Emax=999_999, Emin=-999_999,
                           traps=[decimal.InvalidOperation, decimal.Overflow, decimal.Inexact, decimal.Rounded])


def validate(config):
    if type(config) is not dict or not {'mode', 'epsilon'} <= config.keys() <= {'mode', 'epsilon', 'marginPercent'}:
        raise ValueError('invalid error judge')
    if type(config['mode']) is not str or config['mode'] not in MODES:
        raise ValueError('invalid error judge mode')
    if type(config['epsilon']) is not str or config['epsilon'] not in EPSILONS:
        raise ValueError('invalid error judge epsilon')
    margin = config.get('marginPercent')
    if margin is not None and (type(margin) is not str or not _MARGIN.fullmatch(margin) or Decimal(margin) > 100):
        raise ValueError('invalid error judge margin')


def _threshold(config):
    """T = epsilon * (1 + margin / 100), exactly."""
    validate(config)
    margin = config.get('marginPercent')
    context = _context()
    scale = context.add(1, context.scaleb(Decimal(DEFAULT_MARGIN if margin is None else margin), -2))
    return context.multiply(Decimal(config['epsilon']), scale)


def _count(data):
    """The number of tokens, from the separator-to-token boundaries of one chunk at a time."""
    count = 0
    after_space = True
    for start in range(0, len(data), _CHUNK):
        classes = data[start:start + _CHUNK].translate(_CLASSES)
        count += classes.count(b' x') + (after_space and classes[0] == 0x78)
        after_space = classes[-1] == 0x20
    return count


def _tokens(data):
    """Yield the tokens in order, holding at most about one chunk of them.

    A token running more than MAX_TOKEN bytes past a chunk is yielded as a memoryview instead of a
    copy, so a single 16 MiB token costs no memory. It is never a number.
    """
    size = len(data)
    start = 0
    while size - start > _CHUNK:
        end = start + _CHUNK
        space = _SPACE.search(data, end)
        stop = space.start() if space else size
        if stop - end <= MAX_TOKEN:
            yield from data[start:stop].split()
        else:
            tokens = data[start:end].split()
            first = end
            if data[end - 1] not in _SPACES:
                first -= len(tokens.pop())
            yield from tokens
            yield memoryview(data)[first:stop]
        start = stop
    yield from data[start:].split()


def matches(actual, expected, config):
    """Whether the actual output matches the expected output under a validated config."""
    if type(actual) is not bytes or type(expected) is not bytes:
        raise TypeError('outputs must be bytes')
    threshold = _threshold(config)
    absolute = config['mode'] != 'relative'
    relative = config['mode'] != 'absolute'
    if _count(actual) != _count(expected):
        return False
    context = _context()
    subtract = context.subtract
    multiply = context.multiply
    number = _NUMBER.fullmatch
    for got, want in zip(_tokens(actual), _tokens(expected), strict=True):
        if got == want:
            continue
        if len(got) > MAX_TOKEN or len(want) > MAX_TOKEN:
            return False
        a = number(got)
        b = number(want)
        if a is None or b is None:
            return False
        if a.lastindex and abs(int(a[1])) > MAX_EXPONENT or b.lastindex and abs(int(b[1])) > MAX_EXPONENT:
            return False
        value = Decimal(want.decode())
        error = subtract(Decimal(got.decode()), value).copy_abs()
        if absolute and error <= threshold or relative and error <= multiply(threshold, value.copy_abs()):
            continue
        return False
    return True
