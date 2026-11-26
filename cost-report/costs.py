"""Turns Cost Explorer results into the rows the running-cost page shows."""

import copy
import datetime
from decimal import ROUND_FLOOR, ROUND_HALF_UP, Decimal
import re

OTHER = "other"
# Cost Explorer keeps about 14 months; asking for more fails the whole call.
READABLE_MONTHS = 12
_PRIVATE = re.compile(r"(?<!\d)\d{12}(?!\d)|arn:")


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


def _cents(amount, rounding):
    return int((amount * 100).to_integral_value(rounding=rounding))


def _rows(amounts, groups):
    """Rounds each row to cents so that the rows add up to the rounded total.

    AWS invoices fees and tax (the rows picked by record type) as lines of their own,
    so those round by themselves and match the invoice. The usage rows share the rest.
    """
    total = _cents(sum(amounts.values(), Decimal(0)), ROUND_HALF_UP)
    order = [group["id"] for group in groups if group["id"] in amounts]
    billed_alone = {group["id"] for group in groups if group.get("record_types")}
    cents = {
        name: _cents(amounts[name], ROUND_HALF_UP if name in billed_alone else ROUND_FLOOR)
        for name in order
    }
    usage = [name for name in order if name not in billed_alone]
    left = total - sum(cents.values())
    if usage:
        # sorted() is stable, so rows with equal remainders keep the configured order.
        by_remainder = sorted(usage, key=lambda name: cents[name] - amounts[name] * 100)
        step = 1 if left > 0 else -1
        takers = by_remainder if left > 0 else by_remainder[::-1]
        for index in range(abs(left)):
            cents[takers[index % len(takers)]] += step
    elif order:
        cents[max(order, key=lambda name: abs(cents[name]))] += left
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


def assert_public(text):
    """Refuses text that carries an AWS account ID or an ARN."""
    if _PRIVATE.search(text):
        raise ValueError("the output contains an account ID or an ARN")
