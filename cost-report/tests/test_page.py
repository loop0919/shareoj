import copy
from pathlib import Path
import re
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import costs
import page

CONFIG = {
    "start": "2026-08",
    "sponsors_url": None,
    "standing_note": "メインのサーバー代は前払いしました。",
    "month_notes": {"2026-10": "3年分のサーバー前払い {prepay} を含みます。"},
    "groups": [
        {"id": "prepay", "label": "サーバーの前払い", "record_types": ["SavingsPlanUpfrontFee"]},
        {"id": "database", "label": "データベース", "services": ["Amazon Relational Database Service"]},
        {"id": "domain", "label": "ドメイン", "services": ["Amazon Route 53"]},
        {"id": "other", "label": "その他"},
        {"id": "tax", "label": "消費税", "record_types": ["Tax"]},
    ],
    "columns": [
        {"label": "サーバー利用料", "groups": ["database", "other"]},
        {"label": "前払い", "groups": ["prepay"]},
        {"label": "ドメイン", "groups": ["domain"]},
        {"label": "消費税", "groups": ["tax"]},
    ],
}

DATA = {
    "version": 1,
    "updated": "2026-12-08",
    "months": {
        "2026-08": {"total": "51.20", "rows": [
            {"group": "database", "amount": "40.00"},
            {"group": "other", "amount": "6.55"},
            {"group": "tax", "amount": "4.65"},
        ]},
        "2026-09": {"total": "64.10", "rows": [
            {"group": "database", "amount": "47.27"},
            {"group": "domain", "amount": "11.00"},
            {"group": "tax", "amount": "5.83"},
        ]},
        "2026-10": {"total": "412.35", "rows": [
            {"group": "prepay", "amount": "318.00"},
            {"group": "database", "amount": "56.85"},
            {"group": "tax", "amount": "37.50"},
        ]},
    },
}


def render(data=DATA, income=None, **overrides):
    config = copy.deepcopy(CONFIG)
    config.update(overrides)
    return page.render(data, config, income or {})


def only(months):
    return {**DATA, "months": months}


def body_rows(html):
    """Each month's row as [month, income, expense columns..., balance], in page order."""
    body = html[html.index("<tbody>"):html.index("</tbody>")]
    found = []
    for row in re.findall(r"<tr>(.*?)</tr>", body, flags=re.S):
        cells = re.findall(r"<t[hd][^>]*>(.*?)</t[hd]>", row, flags=re.S)
        found.append([re.sub(r"<[^>]+>", "", cell) for cell in cells])
    return found


class MoneyTest(unittest.TestCase):
    def test_money(self):
        self.assertEqual(page.money("1234.5"), "$1,234.50")
        self.assertEqual(page.money("-12.34"), "-$12.34")
        self.assertEqual(page.money("0.00"), "$0.00")
        self.assertEqual(page.money("412.35"), "$412.35")


class LedgerTest(unittest.TestCase):
    def test_one_row_a_month_newest_first_with_expenses_by_column(self):
        self.assertEqual(body_rows(render(standing_note=None, month_notes={})), [
            ["2026年10月", "", "$56.85", "$318.00", "", "$37.50", "-$527.65"],
            ["2026年9月", "", "$47.27", "", "$11.00", "$5.83", "-$115.30"],
            ["2026年8月", "", "$46.55", "", "", "$4.65", "-$51.20"],
        ])

    def test_column_headings_follow_the_settings(self):
        head = render()[render().index("<thead>"):render().index("</thead>")]
        labels = re.findall(r"<th[^>]*>(.*?)</th>", head)
        self.assertEqual(labels, ["月", "収入", "支出", "累計収支", "GitHub Sponsors", "サーバー利用料", "前払い", "ドメイン", "消費税"])
        self.assertIn('<th colspan="4" scope="colgroup">支出</th>', head)

    def test_income_is_added_to_the_balance_from_its_month_on(self):
        rows = body_rows(render(income={"2026-09": "30.00"}, standing_note=None, month_notes={}))
        self.assertEqual([row[1] for row in rows], ["", "$30.00", ""])
        self.assertEqual([row[-1] for row in rows], ["-$497.65", "-$85.30", "-$51.20"])

    def test_income_of_a_month_without_final_costs_is_not_shown(self):
        with_later = render(income={"2026-11": "10.00"})
        self.assertEqual(body_rows(with_later), body_rows(render()))
        self.assertNotIn("$10.00", with_later)

    def test_balance_above_zero_has_no_minus(self):
        rows = body_rows(render(income={"2026-08": "100.00"}))
        self.assertEqual(rows[-1][-1], "$48.80")

    def test_credit_month_shows_a_negative_expense_and_raises_the_balance(self):
        html = render(only({"2026-10": {"total": "-20.00", "rows": [{"group": "database", "amount": "-20.00"}]}}), month_notes={})
        self.assertEqual(body_rows(html), [["2026年10月", "", "-$20.00", "", "", "", "$20.00"]])

    def test_row_of_a_removed_group_joins_the_column_that_holds_the_catch_all(self):
        html = render(only({"2026-10": {"total": "5.00", "rows": [
            {"group": "removed", "amount": "3.00"},
            {"group": "other", "amount": "2.00"},
        ]}}), month_notes={})
        self.assertEqual(body_rows(html), [["2026年10月", "", "$5.00", "", "", "", "-$5.00"]])

    def test_table_scrolls_inside_a_region_keyboard_users_can_reach(self):
        self.assertIn('<div class="ledger" role="region" aria-label="月ごとの収支" tabindex="0">', render())


class NotesTest(unittest.TestCase):
    def test_month_notes_are_numbered_oldest_first_and_marked_on_their_rows(self):
        html = render(month_notes={
            "2026-08": "移行の費用を含みます。",
            "2026-10": "3年分のサーバー前払い {prepay} を含みます。",
        })
        rows = body_rows(html)
        self.assertEqual([row[0] for row in rows], ["2026年10月※2", "2026年9月", "2026年8月※1"])
        notes = re.findall(r"<li>(.*?)</li>", html[html.index('<ul class="notes">'):])
        self.assertEqual(notes, [
            "メインのサーバー代は前払いしました。",
            "※1 移行の費用を含みます。",
            "※2 3年分のサーバー前払い $318.00 を含みます。",
        ])

    def test_prepayment_without_a_note_gets_no_mark(self):
        html = render(month_notes={}, standing_note=None)
        self.assertNotIn("※", html)
        self.assertNotIn('<ul class="notes">', html)

    def test_note_of_a_month_that_is_not_shown_is_left_out(self):
        html = render(month_notes={"2027-01": "まだ先の月です。"}, standing_note=None)
        self.assertNotIn("まだ先の月", html)
        self.assertNotIn("※", html)

    def test_standing_note_alone(self):
        html = render(month_notes={})
        self.assertEqual(re.findall(r"<li>(.*?)</li>", html), ["メインのサーバー代は前払いしました。"])

    def test_text_is_escaped(self):
        html = render(standing_note="a < b & c", month_notes={"2026-09": "<b>x</b>"})
        self.assertIn("a &lt; b &amp; c", html)
        self.assertIn("※1 &lt;b&gt;x&lt;/b&gt;", html)
        self.assertNotIn("<b>x</b>", html)


class PageTest(unittest.TestCase):
    def test_sponsors_link_only_when_set(self):
        self.assertNotIn("GitHub SponsorsでShareOJを支援する", render())
        html = render(sponsors_url="https://example.com/s")
        self.assertIn('<a href="https://example.com/s">GitHub SponsorsでShareOJを支援する</a>', html)

    def test_updated_date_in_japanese(self):
        self.assertIn("最終更新日: 2026年12月8日", render())

    def test_no_months(self):
        html = render({"version": 1, "updated": None, "months": {}}, income={"2026-09": "30.00"})
        self.assertIn("まだ載せられる月がありません。", html)
        self.assertNotIn("<table", html)
        self.assertNotIn("最終更新日", html)

    def test_page_basics(self):
        html = render()
        self.assertTrue(html.startswith("<!doctype html>"))
        self.assertIn('<html lang="ja">', html)
        self.assertIn('<meta name="viewport"', html)
        self.assertIn("<title>ShareOJの運用費</title>", html)
        self.assertIn("金額は米ドルです。", html)
        # The page states the balance and leaves its reading to the reader.
        self.assertNotIn("運営者が自分で負担", html)
        for banned in ("noindex", "<script", "http://", "${"):
            self.assertNotIn(banned, html)

    def test_output_is_public(self):
        costs.assert_public(render(income={"2026-09": "30.00"}))


if __name__ == "__main__":
    unittest.main()
