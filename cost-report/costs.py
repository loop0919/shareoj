"""Turns Cost Explorer results into the rows the running-cost page shows."""

import copy
import datetime
from decimal import ROUND_HALF_UP, Decimal, InvalidOperation
import re

OTHER = "other"
# Cost Explorer keeps about 14 months; asking for more fails the whole call.
READABLE_MONTHS = 12
_PRIVATE = re.compile(r"(?<!\d)\d{12}(?!\d)|arn:")
_MONTH = re.compile(r"\d{4}-(0[1-9]|1[0-2])")


class Pending(Exception):
    """AWS has not finalized these months, so nothing may be published yet."""

    def __init__(self, months):
        super().__init__("not finalized: " + ", ".join(months))
        self.months = months


def _first_day(year, month):
    # month may run past either end of the year
    return datetime.date(year + (month - 1) // 12, (month - 1) % 12 + 1, 1)


def query_period(start, today):
    """Returns the (Start, End) dates to ask for, or None before the first month has ended."""
    end = _first_day(today.year, today.month)
    first = max(
        datetime.date.fromisoformat(start + "-01"),
        _first_day(today.year, today.month - READABLE_MONTHS),
    )
    if first >= end:
        return None
    return first.isoformat(), end.isoformat()


def _group_of(service, record_type, groups):
    for group in groups:
        if record_type in group.get("record_types", ()):
            return group["id"]
    for group in groups:
        if service in group.get("services", ()):
            return group["id"]
    return OTHER


def _dollars(cents):
    sign = "-" if cents < 0 else ""
    return f"{sign}{abs(cents) // 100}.{abs(cents) % 100:02d}"


def _cents(amount):
    return int((amount * 100).to_integral_value(rounding=ROUND_HALF_UP))


def _rows(amounts, groups):
    """Rounds each row to cents so that the rows add up to the rounded total.

    Every row rounds on its own. When the rows then miss the total by a cent or two,
    the usage rows make it up: AWS invoices fees and tax (the rows picked by record type)
    as lines of their own, so those must keep the invoiced amount. A row at zero is not
    shown, so it never gives or takes a cent while a shown row can.
    """
    total = _cents(sum(amounts.values(), Decimal(0)))
    order = [group["id"] for group in groups if group["id"] in amounts]
    billed_alone = {group["id"] for group in groups if group.get("record_types")}
    cents = {name: _cents(amounts[name]) for name in order}
    left = total - sum(cents.values())
    shown_usage = [name for name in order if name not in billed_alone and cents[name]]
    shown = [name for name in order if cents[name]]
    if left and not shown_usage and shown:
        cents[max(shown, key=lambda name: abs(cents[name]))] += left
    elif left:
        # Take a cent back from the rows that rounded up the most, or give one to those that
        # rounded down the most. Ties fall on the later row when taking, the earlier when giving.
        takers = sorted(
            shown_usage or order,
            key=lambda name: (cents[name] - amounts[name] * 100, order.index(name)),
            reverse=left < 0,
        )
        for index in range(abs(left)):
            cents[takers[index % len(takers)]] += 1 if left > 0 else -1
    return _dollars(total), [{"group": name, "amount": _dollars(cents[name])} for name in order if cents[name]]


def summarize(results, config):
    """Groups every page of ResultsByTime into {month: {"total", "rows"}}."""
    groups = config["groups"]
    if not any(group["id"] == OTHER for group in groups):
        raise ValueError(f'config needs a group with id "{OTHER}"')
    pending = sorted({result["TimePeriod"]["Start"][:7] for result in results if result.get("Estimated")})
    if pending:
        raise Pending(pending)
    months = {}
    for result in results:
        amounts = months.setdefault(result["TimePeriod"]["Start"][:7], {})
        for entry in result["Groups"]:
            service, record_type = entry["Keys"]
            name = _group_of(service, record_type, groups)
            amounts[name] = amounts.get(name, Decimal(0)) + Decimal(entry["Metrics"]["UnblendedCost"]["Amount"])
    summary = {}
    for month in sorted(months):
        total, rows = _rows(months[month], groups)
        summary[month] = {"total": total, "rows": rows}
    return summary


def merge(stored, fresh, today):
    """Overwrites the months AWS still returns and keeps the older ones."""
    merged = copy.deepcopy(stored)
    merged["months"].update(copy.deepcopy(fresh))
    changed = merged["months"] != stored["months"]
    if changed:
        merged["updated"] = today.isoformat()
    return merged, changed


def income(raw):
    """Checks the hand-written income file, {"2026-12": "25.00"}, and returns amounts with two decimals.

    The file is typed by a person and published without review, so anything odd stops the run.
    """
    if not isinstance(raw, dict):
        raise ValueError("the income file must be an object of month: amount")
    checked = {}
    for month, amount in raw.items():
        if not _MONTH.fullmatch(month):
            raise ValueError(f"income: {month!r} is not a month like 2026-12")
        if isinstance(amount, bool) or not isinstance(amount, (str, int, float)):
            raise ValueError(f"income: the amount of {month} must be a number")
        try:
            value = Decimal(str(amount))
        except InvalidOperation:
            raise ValueError(f"income: the amount of {month} is not a number") from None
        if not value.is_finite() or value < 0 or value != value.quantize(Decimal("0.01")):
            raise ValueError(f"income: the amount of {month} must be zero or more, with at most two decimals")
        checked[month] = f"{value:.2f}"
    return checked


def assert_public(text):
    """Refuses text that carries an AWS account ID or an ARN."""
    if _PRIVATE.search(text):
        raise ValueError("the output contains an account ID or an ARN")
