import random
import re
import sys
import time
import tracemalloc
import base64
import unittest
from fractions import Fraction
from pathlib import Path
from unittest.mock import patch

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / 'runner'))
import tolerance


OMITTED = object()


def config(mode, epsilon, margin=OMITTED):
    value = {'mode': mode, 'epsilon': epsilon}
    if margin is not OMITTED:
        value['marginPercent'] = margin
    return value


ABSOLUTE_6 = config('absolute', '1e-6')
REFERENCE_NUMBER = re.compile(rb'-?([0-9]+|[0-9]+\.[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?')


def reference_number(token):
    """The value of a token under the rules, computed with Fraction only, or None for text."""
    if len(token) > 400:
        return None
    match = REFERENCE_NUMBER.fullmatch(token)
    if match is None:
        return None
    if match[2] is not None and abs(int(match[2][1:])) > 400:
        return None
    return Fraction(token.decode())


def reference(actual, expected, setting):
    """Exact rational arithmetic with no shortcuts: what tolerance.matches must agree with."""
    left = re.findall(rb'[^ \t\n\r\v\f]+', actual)
    right = re.findall(rb'[^ \t\n\r\v\f]+', expected)
    if len(left) != len(right):
        return False
    margin = setting.get('marginPercent')
    threshold = Fraction(setting['epsilon']) * (1 + Fraction('0.1' if margin is None else margin) / 100)
    for a, b in zip(left, right):
        if a == b:
            continue
        x, y = reference_number(a), reference_number(b)
        if x is None or y is None:
            return False
        error = abs(x - y)
        absolute = error <= threshold
        relative = error <= threshold * abs(y)
        if not {'absolute': absolute, 'relative': relative, 'either': absolute or relative}[setting['mode']]:
            return False
    return True


def spell(coefficient, exponent, rng):
    """Write coefficient * 10**exponent exactly, in one of the forms the rules read as a number."""
    sign = '-' if coefficient < 0 or (coefficient == 0 and rng.random() < 0.2) else ''
    digits = str(abs(coefficient))
    style = rng.randrange(4)
    if style == 0:
        mark = rng.choice('eE')
        power = exponent + len(digits) - 1
        text = digits[0] + ('.' + digits[1:] if len(digits) > 1 else rng.choice(['', '.']))
        return sign + text + mark + ('+' if power >= 0 and rng.random() < 0.5 else '') + str(power)
    if style == 1:
        return sign + digits + rng.choice('eE') + str(exponent)
    if exponent >= 0:
        return sign + digits + '0' * exponent + rng.choice(['', '.', '.0'])
    digits = digits.rjust(1 - exponent, '0')
    whole, fraction = digits[:exponent], digits[exponent:]
    if whole == '0' and rng.random() < 0.3:
        whole = ''
    return sign + whole + '.' + fraction


def add(left, right):
    (c1, e1), (c2, e2) = left, right
    e = min(e1, e2)
    return c1 * 10 ** (e1 - e) + c2 * 10 ** (e2 - e), e


class ToleranceTests(unittest.TestCase):
    def assertMatches(self, actual, expected, setting, result):
        self.assertIs(tolerance.matches(actual, expected, setting), result, (actual[:60], expected[:60], setting))
        self.assertIs(reference(actual, expected, setting), result, 'the reference disagrees with the test')

    def test_constants(self):
        self.assertEqual(tolerance.EPSILONS, tuple(f'1e-{k}' for k in range(1, 13)))
        self.assertEqual(tolerance.DEFAULT_MARGIN, '0.1')

    def test_boundary_is_exact_in_each_mode(self):
        absolute = config('absolute', '1e-6', '0')
        self.assertMatches(b'1.000001', b'1', absolute, True)
        self.assertMatches(b'0.999999', b'1', absolute, True)
        self.assertMatches(b'1.0000010000000001', b'1', absolute, False)
        relative = config('relative', '1e-6', '0')
        self.assertMatches(b'1000.001', b'1000', relative, True)
        self.assertMatches(b'-1000.001', b'-1000', relative, True)
        self.assertMatches(b'1000.0010000001', b'1000', relative, False)
        either = config('either', '1e-6', '0')
        self.assertMatches(b'0.500001', b'0.5', either, True)
        self.assertMatches(b'1000.001', b'1000', either, True)
        self.assertMatches(b'1000.0011', b'1000', either, False)
        self.assertMatches(b'0.500001', b'0.5', config('relative', '1e-6', '0'), False)
        self.assertMatches(b'1000.001', b'1000', absolute, False)

    def test_margin_widens_the_threshold(self):
        self.assertMatches(b'0.000001001', b'0', ABSOLUTE_6, True)
        self.assertMatches(b'0.0000010010000001', b'0', ABSOLUTE_6, False)
        self.assertMatches(b'0.000001001', b'0', config('absolute', '1e-6', None), True)
        self.assertMatches(b'0.0000010010000001', b'0', config('absolute', '1e-6', None), False)
        self.assertMatches(b'0.000001001', b'0', config('absolute', '1e-6', '0.1'), True)
        self.assertMatches(b'0.000002', b'0', config('absolute', '1e-6', '100'), True)
        self.assertMatches(b'0.0000020000001', b'0', config('absolute', '1e-6', '100'), False)
        self.assertMatches(b'0.000001001', b'0', config('absolute', '1e-6', '0'), False)
        self.assertMatches(b'0.000001', b'0', config('absolute', '1e-6', '0'), True)

    def test_decimal_values_are_not_rounded_through_floats(self):
        setting = config('absolute', '1e-1', '0')
        self.assertMatches(b'0.2', b'0.3', setting, True)
        self.assertMatches(b'0.19999999999999999', b'0.3', setting, False)
        # Floats cannot tell these apart: the second pair passes a float comparison.
        self.assertLessEqual(abs(0.3 - float('0.19999999999999999')), 0.1)
        self.assertGreater(abs(float('0.000001001')), 1e-6 * 1.001)

    def test_number_forms(self):
        for actual, expected in ((b'3', b'3.0'), (b'-0.5', b'-.5'), (b'2.', b'2'), (b'.25', b'0.25'),
                                 (b'1e-3', b'0.001'), (b'6.02E+23', b'602000000000000000000000'),
                                 (b'-0', b'0'), (b'0.0', b'0'), (b'2.', b'2.0000001'), (b'.25', b'0.2500001')):
            with self.subTest(actual=actual, expected=expected):
                self.assertMatches(actual, expected, ABSOLUTE_6, True)
                self.assertMatches(expected, actual, ABSOLUTE_6, True)
        for actual in (b'2.', b'2.0', b'2e0'):
            self.assertMatches(actual, b'2', ABSOLUTE_6, True)
        self.assertMatches(b'1e3', b'1000', ABSOLUTE_6, True)
        for actual in (b'-0', b'-0.0'):
            self.assertMatches(actual, b'0', ABSOLUTE_6, True)
        for text, number in ((b'+1.0', b'1.0'), (b'0x10', b'16'), (b'1_000', b'1000'), (b'nan', b'0'),
                             (b'NaN', b'0'), (b'inf', b'1e300'), (b'-inf', b'-1e300'),
                             (b'infinity', b'1e300'), (b'1e', b'1'), (b'e5', b'1e5'), (b'.', b'0'),
                             (b'-', b'0'), (b'1.2.3', b'1.2')):
            with self.subTest(text=text):
                self.assertMatches(text, text, ABSOLUTE_6, True)
                self.assertMatches(text, number, config('either', '1e-1'), False)
                self.assertMatches(number, text, config('either', '1e-1'), False)
        self.assertMatches(b'nan', b'nan', ABSOLUTE_6, True)
        self.assertMatches(b'+1.0', b'1.0', ABSOLUTE_6, False)
        self.assertMatches(b'Infinity', b'inf', ABSOLUTE_6, False)
        self.assertMatches(b'NaN', b'nan', ABSOLUTE_6, False)

    def test_oversized_tokens_are_compared_as_text(self):
        either = config('either', '1e-1')
        long = b'1' * 400 + b'0'
        self.assertMatches(long, long, either, True)
        self.assertMatches(long[:-1] + b'1', long, either, False)
        self.assertMatches(b'1' * 399 + b'1', b'1' * 399 + b'0', either, True)
        self.assertMatches(b'1e401', b'1e401', ABSOLUTE_6, True)
        self.assertMatches(b'1e401', b'1.0e401', ABSOLUTE_6, False)
        self.assertMatches(b'1e400', b'1.0e400', ABSOLUTE_6, True)
        self.assertMatches(b'1E-0400', b'0', ABSOLUTE_6, True)
        self.assertMatches(b'1e-401', b'0', ABSOLUTE_6, False)
        self.assertMatches(b'1e-400', b'0', ABSOLUTE_6, True)
        self.assertMatches(b'1e' + b'0' * 397 + b'1', b'10', ABSOLUTE_6, True)
        self.assertMatches(b'1e' + b'0' * 398 + b'1', b'10', ABSOLUTE_6, False)

    def test_expected_zero_in_relative_mode(self):
        relative = config('relative', '1e-6')
        for actual in (b'0', b'0.0', b'-0'):
            self.assertMatches(actual, b'0', relative, True)
        self.assertMatches(b'1e-300', b'0', relative, False)
        self.assertMatches(b'1e-300', b'0', config('either', '1e-6'), True)

    def test_text_tokens_need_exact_match(self):
        self.assertMatches(b'Yes 3.1400001', b'Yes 3.14\n', ABSOLUTE_6, True)
        self.assertMatches(b'yes 3.14', b'Yes 3.14\n', ABSOLUTE_6, False)
        self.assertMatches(b'3.14 Yes', b'Yes 3.14\n', ABSOLUTE_6, False)
        self.assertMatches(b'1\n2', b'1 2\n', ABSOLUTE_6, True)
        self.assertMatches(b'1\r\n2\r\n', b'1\n2\n', ABSOLUTE_6, True)
        self.assertMatches(b'1 2 \t\n', b'1 2', ABSOLUTE_6, True)
        self.assertMatches(b'\x00 \xff\xfe 1', b'\x00 \xff\xfe 1.0000001', ABSOLUTE_6, True)
        self.assertMatches(b'\x00', b'0', ABSOLUTE_6, False)
        self.assertMatches(b'1\xa02', b'1 2', ABSOLUTE_6, False)
        for actual, expected in (('1', b'1'), (b'1', '1'), (bytearray(b'1'), b'1')):
            with self.assertRaises(TypeError):
                tolerance.matches(actual, expected, ABSOLUTE_6)

    def test_token_count_mismatch_is_rejected_before_comparing_numbers(self):
        self.assertMatches(b'1 1', b'1', ABSOLUTE_6, False)
        self.assertMatches(b'', b'1', ABSOLUTE_6, False)
        self.assertMatches(b' \n', b'', ABSOLUTE_6, True)
        started = time.perf_counter()
        self.assertIs(tolerance.matches(b'1.0 ' * 2_000_000, b'1', ABSOLUTE_6), False)
        elapsed = time.perf_counter() - started
        self.assertLess(elapsed, 1, f'{elapsed:.3f}s')

    def test_validate_rejects_malformed_config(self):
        tolerance.validate({'mode': 'either', 'epsilon': '1e-6'})
        for mode in ('absolute', 'relative', 'either'):
            for epsilon in tolerance.EPSILONS:
                tolerance.validate({'mode': mode, 'epsilon': epsilon})
        for margin in ('0', '0.5', '100', '12.345678', '100.000000', '000', None):
            tolerance.validate({'mode': 'either', 'epsilon': '1e-6', 'marginPercent': margin})
        for bad in (None, {}, [], 'either', {'mode': 'both', 'epsilon': '1e-6'}, {'epsilon': '1e-6'},
                    {'mode': 'either'}, {'mode': 'either', 'epsilon': '1e-6', 'extra': 1}):
            with self.subTest(config=bad), self.assertRaises(ValueError):
                tolerance.validate(bad)
        for epsilon in ('1e-13', '1e-0', '0.000001', 1e-6, '1E-6', None):
            with self.subTest(epsilon=epsilon), self.assertRaises(ValueError):
                tolerance.validate({'mode': 'either', 'epsilon': epsilon})
        for margin in ('100.000001', '101', '-1', '1e1', '.5', '0.1234567', 0.1, 0, '', '1.', ' 1', '1\n', '٣'):
            with self.subTest(margin=margin), self.assertRaises(ValueError):
                tolerance.validate({'mode': 'either', 'epsilon': '1e-6', 'marginPercent': margin})
        with self.assertRaises(ValueError):
            tolerance.matches(b'1', b'1', {'mode': 'both', 'epsilon': '1e-6'})

    def test_fast_path_agrees_with_fractions(self):
        rng = random.Random(20261112)
        results = {True: 0, False: 0}
        for _ in range(20_000):
            mode = rng.choice(('absolute', 'relative', 'either'))
            k = rng.choice((3, 6, 12))
            margin = rng.choice(('0', '0.1'))
            setting = config(mode, f'1e-{k}', margin)
            threshold = (10**8 + (100_000 if margin == '0.1' else 0), -k - 8)
            kind = rng.randrange(5)
            if kind == 0:
                expected = (0, 0)
            elif kind == 1:
                expected = (rng.randint(-10**4, 10**4), -rng.randint(0, 6))
            elif kind == 2:
                expected = (rng.randint(-10**60, 10**60), -rng.randint(0, 80))
            elif kind == 3:
                expected = (rng.randint(-10**15, 10**15), rng.randint(-300, 280))
            else:
                expected = (rng.randint(-10**8, 10**8), rng.randint(-30, 30))
            relative = mode == 'relative' or (mode == 'either' and rng.random() < 0.5)
            if relative:
                distance = (threshold[0] * abs(expected[0]), threshold[1] + expected[1])
            else:
                distance = threshold
            unit = (1, min(distance[1], expected[1]) - rng.randint(0, 3))
            step = rng.randrange(6)
            if step < 3:
                distance = add(distance, (unit[0] * (step - 1), unit[1]))
            elif step == 3:
                distance = (distance[0] * rng.randint(0, 20), distance[1] - 1)
            elif step == 4:
                distance = unit
            if rng.random() < 0.5:
                distance = (-distance[0], distance[1])
            actual = add(expected, distance)
            a = spell(*actual, rng).encode()
            b = spell(*expected, rng).encode()
            want = reference(a, b, setting)
            results[want] += 1
            self.assertIs(tolerance.matches(a, b, setting), want, (a, b, setting))
        self.assertGreater(min(results.values()), 3000, results)

    def test_chunk_boundaries_do_not_change_tokens(self):
        rng = random.Random(7)
        pieces = [b'1', b'1.0', b'0.9999999', b'x', b'-', b'1' * 401, b'1' * 400 + b'2', b'2' * 1200,
                  b'1e-400', b'\x00\xff']
        for size in (1, 2, 3, 7, 64):
            with patch.object(tolerance, '_CHUNK', size):
                for _ in range(300):
                    count = rng.randint(0, 12)
                    tokens = [rng.choice(pieces) for _ in range(count)]
                    other = [rng.choice(pieces) if rng.random() < 0.2 else token for token in tokens]
                    if rng.random() < 0.1:
                        other.append(b'1')
                    spaces = lambda: b''.join(rng.choice([b' ', b'\n', b'\r\n', b'\t', b'\v\f'])
                                              for _ in range(rng.randint(1, 3)))
                    a = (spaces() if rng.random() < 0.5 else b'') + spaces().join(tokens)
                    b = spaces().join(other) + (spaces() if rng.random() < 0.5 else b'')
                    with self.subTest(size=size, a=a[:80], b=b[:80]):
                        self.assertIs(tolerance.matches(a, b, ABSOLUTE_6), reference(a, b, ABSOLUTE_6))

    def test_extra_memory_does_not_grow_with_the_output(self):
        huge = b'7' * (16 << 20)
        other = huge[:-1] + b'8'
        prefixed = b'1 ' + huge
        prefixed_close = b'1.0000001 ' + huge
        words = b'ab ' * 1_000_000
        lines = b'ab\n' * 1_000_000
        many = b'0.5 ' * 100_000
        close = b'0.5000001 ' * 100_000
        # tracemalloc slows each allocation down, so the numbers are fewer than the words.
        tracemalloc.start()
        try:
            self.assertIs(tolerance.matches(huge, huge, ABSOLUTE_6), True)
            self.assertIs(tolerance.matches(other, huge, ABSOLUTE_6), False)
            self.assertIs(tolerance.matches(prefixed, prefixed_close, ABSOLUTE_6), True)
            self.assertIs(tolerance.matches(words, lines, ABSOLUTE_6), True)
            self.assertIs(tolerance.matches(many, close, ABSOLUTE_6), True)
            _, peak = tracemalloc.get_traced_memory()
        finally:
            tracemalloc.stop()
        self.assertLess(peak, 1 << 20, f'peak {peak} bytes')

    def test_million_boundary_pairs_finish_in_time(self):
        expected = b'0\n' * 1_000_000
        actual = b'0.000001001\n' * 1_000_000
        started = time.perf_counter()
        self.assertIs(tolerance.matches(actual, expected, ABSOLUTE_6), True)
        elapsed = time.perf_counter() - started
        self.assertLess(elapsed, 30, f'{elapsed:.2f}s')



class ErrorJudgeHostTests(unittest.TestCase):
    """The worker compares with tolerance.matches only for jobs that carry errorJudge."""

    def run_job(self, printed, expected='0.5\n', error_judge=None, **changes):
        from test_scoring import Judge, finished
        request = dict(runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='submitted source',
                       memoryLimitMb=256, timeLimitMs=1000, cases=[dict(name='c0', input='0', output=expected)])
        if error_judge is not None:
            request['errorJudge'] = error_judge
        request.update(changes)
        reply = finished(output=base64.b64encode(printed.encode()).decode())
        return Judge(self, submission=lambda index: reply)(request)

    def test_jobs_reject_malformed_or_conflicting_error_judge(self):
        from test_scoring import host
        from test_run_twice import twice_job
        base = twice_job()
        del base['runTwice']
        judge = dict(mode='absolute', epsilon='1e-6')
        code = dict(runtime='python314', source='x')
        host.validate_job(dict(base, errorJudge=judge), 'sha256:test')
        for conflict in (dict(checker=code), dict(interactor=code), dict(runTwice=code), dict(generate=True), dict(validate=True)):
            with self.subTest(conflict=list(conflict)), self.assertRaises(ValueError):
                host.validate_job(dict(base, errorJudge=judge, **conflict), 'sha256:test')
        with self.assertRaisesRegex(ValueError, 'conflicting judge modes'):
            host.validate_job(dict(base, errorJudge=judge, runTwice=code), 'sha256:test')
        for bad in (dict(judge, epsilon='1e-13'), dict(judge, mode='x'), {}, 'absolute', dict(judge, extra=1)):
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                host.validate_job(dict(base, errorJudge=bad), 'sha256:test')

    def test_host_compares_with_tolerance(self):
        judge = dict(mode='absolute', epsilon='1e-6')
        self.assertEqual(self.run_job('0.5000001\n', error_judge=judge)['verdict'], 'AC')
        self.assertEqual(self.run_job('0.5000001\n')['verdict'], 'WA')
        result = self.run_job('0.50001\n', error_judge=judge)
        self.assertEqual((result['verdict'], result['passed']), ('WA', 0))

    def test_results_of_error_judge_jobs_carry_the_mark(self):
        from test_scoring import Judge
        from test_run_twice import twice_job
        judge = dict(mode='absolute', epsilon='1e-6')
        self.assertIs(self.run_job('0.5\n', error_judge=judge).get('errorJudge'), True)
        self.assertIs(self.run_job('0.6\n', error_judge=judge).get('errorJudge'), True)
        ce = Judge(self, compiled={'submitted source': False})(dict(
            runtime='cpp23-gcc-isolate', runtimeDigest='sha256:test', source='submitted source', memoryLimitMb=256, timeLimitMs=1000,
            cases=[dict(input='0', output='0')], errorJudge=judge))
        self.assertEqual((ce['verdict'], ce.get('errorJudge')), ('CE', True))
        self.assertNotIn('errorJudge', self.run_job('0.5\n'))
        self.assertNotIn('errorJudge', self.run_job('0.6\n'))
        twice = Judge(self, compiled={'submitted source': False})(twice_job())
        self.assertEqual((twice['verdict'], twice.get('runTwice'), 'errorJudge' in twice), ('CE', True, False))

    def test_sample_check_keeps_sample_details(self):
        result = self.run_job('0.5000001\n', error_judge=dict(mode='absolute', epsilon='1e-6'), easyTest=True)
        details = result['cases'][0]['sampleDetails']
        self.assertEqual((result['verdict'], details['input']['text'], details['expectedOutput']['text'], details['actualOutput']['text']),
                         ('AC', '0\n', '0.5\n', '0.5000001\n'))

    def test_group_scoring_uses_tolerance_verdicts(self):
        from test_scoring import scoring_job
        request = scoring_job(groups=(('a', 100),), members=(['a'], ['a']), errorJudge=dict(mode='relative', epsilon='1e-3'))
        for case in request['cases']:
            case['output'] = '1000\n'
        result = self.run_job('1000.5\n', **{key: request[key] for key in ('cases', 'scoring', 'errorJudge')})
        self.assertEqual((result['verdict'], result['passed']), ('AC', 2))
        self.assertEqual(result['scoring']['score'], 10000)
        result = self.run_job('1002\n', **{key: request[key] for key in ('cases', 'scoring', 'errorJudge')})
        self.assertEqual((result['verdict'], result['scoring']['score']), ('WA', 0))

    def test_local_runner_rejects_error_judge_jobs(self):
        import io
        import json
        import importlib.util
        spec = importlib.util.spec_from_file_location('local_runner', Path(__file__).resolve().parents[1] / 'local.py')
        local = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(local)
        request = dict(runtime='python314-local', errorJudge=dict(mode='absolute', epsilon='1e-6'))
        with patch('sys.stdin', io.StringIO(json.dumps(request) + '\n')), self.assertRaisesRegex(ValueError, '誤差ジャッジはisolateワーカー'):
            local.main()


if __name__ == '__main__':
    unittest.main()
