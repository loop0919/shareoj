"""Builds the running-cost page from the stored months."""

from decimal import Decimal
import html
from pathlib import Path
import string

import costs

TEMPLATE = Path(__file__).with_name("template.html")
AUTO_NOTE = "サーバーの前払い {prepay} を含みます。"
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


def _rows(month, config):
    """Returns (group, amount) in the configured order; rows of removed groups join the catch-all."""
    known = {group["id"] for group in config["groups"]}
    amounts = {}
    for row in month["rows"]:
        name = row["group"] if row["group"] in known else costs.OTHER
        amounts[name] = amounts.get(name, Decimal(0)) + Decimal(row["amount"])
    return [(group, amounts[group["id"]]) for group in config["groups"] if group["id"] in amounts]


def _note(key, rows, config):
    prepay = next((amount for group, amount in rows if group["id"] == PREPAY), None)
    text = config.get("month_notes", {}).get(key)
    if text is None and prepay:
        text = AUTO_NOTE
    if text is None:
        return ""
    filled = text.replace("{prepay}", money(prepay or 0))
    return f'<p class="note">{html.escape(filled)}</p>'


def _table(rows, total):
    lines = ['<table>', '<thead><tr><th scope="col">内訳</th><th scope="col">金額</th></tr></thead>', "<tbody>"]
    for group, amount in rows:
        aws = f'<span class="aws">{html.escape(group["aws"])}</span>' if group.get("aws") else ""
        lines.append(f'<tr><th scope="row">{html.escape(group["label"])}{aws}</th><td>{money(amount)}</td></tr>')
    lines += ["</tbody>", f'<tfoot><tr><th scope="row">合計</th><td>{money(total)}</td></tr></tfoot>', "</table>"]
    return "\n".join(lines)


def _latest(key, month, config):
    rows = _rows(month, config)
    standing = config.get("standing_note")
    return "\n".join(filter(None, [
        '<section class="latest">',
        f'<p class="month">{_month_label(key)}<span class="basis">AWSの請求額・税込</span></p>',
        f'<p class="total">{money(month["total"])}</p>',
        f'<p class="note">{html.escape(standing)}</p>' if standing else "",
        _note(key, rows, config),
        _table(rows, month["total"]),
        "</section>",
    ]))


def _past(keys, months, config):
    if not keys:
        return ""
    lines = ['<section class="past">', "<h2>これまでの月</h2>"]
    for key in keys:
        rows = _rows(months[key], config)
        lines += [
            "<details>",
            f'<summary><span>{_month_label(key)}</span><span>{money(months[key]["total"])}</span></summary>',
            '<div class="body">',
            _note(key, rows, config),
            _table(rows, months[key]["total"]),
            "</div>",
            "</details>",
        ]
    lines.append("</section>")
    return "\n".join(filter(None, lines))


def render(data, config):
    months = data["months"]
    keys = sorted(months, reverse=True)
    if keys:
        latest = _latest(keys[0], months[keys[0]], config)
        updated = f'<p>最終更新日: {_date_label(data["updated"])}</p>' if data.get("updated") else ""
    else:
        latest = '<p class="empty">まだ載せられる月がありません。</p>'
        updated = ""
    url = config.get("sponsors_url")
    sponsors = f'<p><a href="{html.escape(url, quote=True)}">GitHub SponsorsでShareOJを支援する</a></p>' if url else ""
    return string.Template(TEMPLATE.read_text(encoding="utf-8")).substitute(
        latest=latest, past=_past(keys[1:], months, config), updated=updated, sponsors=sponsors,
    )
