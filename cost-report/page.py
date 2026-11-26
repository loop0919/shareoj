"""Builds the running-cost page: one row a month with income, expenses and the running balance (累計収支)."""

from decimal import Decimal
import html
from pathlib import Path
import string

import costs

TEMPLATE = Path(__file__).with_name("template.html")
PREPAY = "prepay"


def money(amount):
    value = Decimal(amount)
    return ("-" if value < 0 else "") + f"${abs(value):,.2f}"


def _month_label(month):
    year, number = month.split("-")
    return f"{year}年{int(number)}月"


def _date_label(day):
    year, month, number = day.split("-")
    return f"{year}年{int(month)}月{int(number)}日"


def _expenses(month, columns):
    """Adds the month's rows up by column; a row of a group no column names joins the catch-all's column."""
    place = {group: index for index, column in enumerate(columns) for group in column["groups"]}
    sums = [Decimal(0)] * len(columns)
    for row in month["rows"]:
        sums[place.get(row["group"], place[costs.OTHER])] += Decimal(row["amount"])
    return sums


def _cell(kind, amount):
    return f'<td class="{kind}">{money(amount) if amount else ""}</td>'


def _ledger(months, config, income):
    """Returns the table's body rows, newest month first, and the notes under the table."""
    columns = config["columns"]
    notes = [config["standing_note"]] if config.get("standing_note") else []
    rows = []
    balance = Decimal(0)
    numbered = 0
    # Oldest first: the balance runs forward, and a note keeps its number when later months arrive.
    for key in sorted(months):
        month = months[key]
        received = Decimal(income.get(key, "0"))
        balance += received - Decimal(month["total"])
        mark = ""
        note = config.get("month_notes", {}).get(key)
        if note is not None:
            numbered += 1
            prepay = sum((Decimal(row["amount"]) for row in month["rows"] if row["group"] == PREPAY), Decimal(0))
            notes.append(f"※{numbered} " + note.replace("{prepay}", money(prepay)))
            mark = f"<sup>※{numbered}</sup>"
        cells = [_cell("income", received)] + [_cell("expense", amount) for amount in _expenses(month, columns)]
        rows.append(f'<tr><th scope="row">{_month_label(key)}{mark}</th>{"".join(cells)}<td class="balance">{money(balance)}</td></tr>')
    return rows[::-1], notes


def _table(body, config):
    columns = config["columns"]
    heads = "".join(f'<th scope="col">{html.escape(column["label"])}</th>' for column in columns)
    return "\n".join([
        '<div class="ledger" role="region" aria-label="月ごとの収支" tabindex="0">',
        "<table>",
        "<thead>",
        f'<tr><th rowspan="2" scope="col">月</th><th scope="col">収入</th><th colspan="{len(columns)}" scope="colgroup">支出</th><th rowspan="2" scope="col">累計収支</th></tr>',
        f'<tr><th scope="col">GitHub Sponsors</th>{heads}</tr>',
        "</thead>",
        "<tbody>",
        *body,
        "</tbody>",
        "</table>",
        "</div>",
    ])


def render(data, config, income):
    months = data["months"]
    if months:
        body, notes = _ledger(months, config, income)
        items = "\n".join(f"<li>{html.escape(note)}</li>" for note in notes)
        ledger = _table(body, config) + (f'\n<ul class="notes">\n{items}\n</ul>' if notes else "")
        updated = f'<p>最終更新日: {_date_label(data["updated"])}</p>' if data.get("updated") else ""
    else:
        ledger = '<p class="empty">まだ載せられる月がありません。</p>'
        updated = ""
    url = config.get("sponsors_url")
    sponsors = f'<p><a href="{html.escape(url, quote=True)}">GitHub SponsorsでShareOJを支援する</a></p>' if url else ""
    return string.Template(TEMPLATE.read_text(encoding="utf-8")).substitute(ledger=ledger, updated=updated, sponsors=sponsors)
