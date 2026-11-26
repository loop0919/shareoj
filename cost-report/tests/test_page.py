import copy
from pathlib import Path
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
        {"id": "prepay", "label": "サーバーの前払い", "aws": "Savings Plans", "record_types": ["SavingsPlanUpfrontFee"]},
        {"id": "database", "label": "データベース", "aws": "Amazon RDS", "services": ["Amazon Relational Database Service"]},
        {"id": "other", "label": "その他"},
        {"id": "tax", "label": "消費税", "record_types": ["Tax"]},
    ],
}

DATA = {
    "version": 1,
    "updated": "2026-12-08",
    "months": {
        "2026-08": {"total": "51.20", "rows": [
            {"group": "database", "amount": "46.55"},
            {"group": "tax", "amount": "4.65"},
        ]},
        "2026-09": {"total": "64.10", "rows": [
            {"group": "database", "amount": "58.27"},
            {"group": "tax", "amount": "5.83"},
        ]},
        "2026-10": {"total": "412.35", "rows": [
            {"group": "prepay", "amount": "318.00"},
            {"group": "database", "amount": "56.85"},
            {"group": "tax", "amount": "37.50"},
        ]},
    },
}


def render(data=DATA, **overrides):
    config = copy.deepcopy(CONFIG)
    config.update(overrides)
    return page.render(data, config)


def only(months):
    return {**DATA, "months": months}


class MoneyTest(unittest.TestCase):
    def test_money(self):
        self.assertEqual(page.money("1234.5"), "$1,234.50")
        self.assertEqual(page.money("-12.34"), "-$12.34")
        self.assertEqual(page.money("0.00"), "$0.00")
        self.assertEqual(page.money("412.35"), "$412.35")


class RenderTest(unittest.TestCase):
    def test_latest_month_is_on_top(self):
        html = render()
        total = '<p class="total">$412.35</p>'
        self.assertEqual(html.count(total), 1)
        self.assertLess(html.index("2026年10月"), html.index(total))
        self.assertLess(html.index(total), html.index("これまでの月"))
        self.assertLess(html.index(total), html.index("<details"))

    def test_past_months_newest_first_in_details(self):
        html = render()
        self.assertEqual(html.count("<details"), 2)
        past = html[html.index("<details"):]
        self.assertLess(past.index("2026年9月"), past.index("2026年8月"))
        self.assertNotIn("2026年10月", past)
        self.assertIn("$64.10", past)
        self.assertIn("$51.20", past)

    def test_rows_use_labels_in_config_order_with_total_last(self):
        latest = render().split("<details")[0]
        positions = [latest.index(label) for label in ("<th scope=\"row\">サーバーの前払い", "データベース", "消費税", "<th scope=\"row\">合計")]
        self.assertEqual(positions, sorted(positions))
        self.assertIn("$318.00", latest)
        self.assertIn("$56.85", latest)
        self.assertIn("$37.50", latest)

    def test_aws_names_are_shown_small(self):
        self.assertIn('<span class="aws">Amazon RDS</span>', render())

    def test_month_note_fills_prepay(self):
        self.assertIn("3年分のサーバー前払い $318.00 を含みます。", render())

    def test_auto_note_when_prepay_without_month_note(self):
        html = render(month_notes={})
        self.assertEqual(html.count("サーバーの前払い $318.00 を含みます。"), 1)
        self.assertNotIn("3年分", html)

    def test_no_note_for_month_without_prepay(self):
        html = render(only({"2026-09": DATA["months"]["2026-09"]}), month_notes={}, standing_note=None)
        self.assertNotIn('class="note"', html)

    def test_note_of_a_past_month_is_inside_its_details(self):
        html = render(month_notes={"2026-09": "ドメインの登録料 $51.00 を含みます。"})
        september = html[html.index("<details"):html.index("</details>")]
        self.assertIn("2026年9月", september)
        self.assertIn('<p class="note">ドメインの登録料 $51.00 を含みます。</p>', september)
        self.assertEqual(html.count("ドメインの登録料"), 1)

    def test_standing_note(self):
        self.assertIn("メインのサーバー代は前払いしました。", render())
        self.assertNotIn("メインのサーバー代", render(standing_note=None))

    def test_sponsors_link_only_when_set(self):
        self.assertNotIn("GitHub Sponsors", render())
        html = render(sponsors_url="https://example.com/s")
        self.assertIn('<a href="https://example.com/s">GitHub SponsorsでShareOJを支援する</a>', html)

    def test_updated_date_in_japanese(self):
        self.assertIn("最終更新日: 2026年12月8日", render())

    def test_single_month(self):
        html = render(only({"2026-10": DATA["months"]["2026-10"]}))
        self.assertIn('<p class="total">$412.35</p>', html)
        self.assertNotIn("これまでの月", html)
        self.assertNotIn("<details", html)

    def test_no_months(self):
        html = render({"version": 1, "updated": None, "months": {}})
        self.assertIn("まだ載せられる月がありません。", html)
        self.assertNotIn("<table", html)
        self.assertNotIn("最終更新日", html)

    def test_negative_amounts(self):
        html = render(only({"2026-10": {"total": "-20.00", "rows": [{"group": "database", "amount": "-20.00"}]}}))
        self.assertIn('<p class="total">-$20.00</p>', html)
        self.assertIn('<td>-$20.00</td>', html)

    def test_unknown_group_id_falls_into_other(self):
        html = render(only({"2026-10": {"total": "5.00", "rows": [
            {"group": "removed", "amount": "3.00"},
            {"group": "other", "amount": "2.00"},
        ]}}))
        self.assertEqual(html.count('<th scope="row">その他'), 1)
        self.assertIn("<td>$5.00</td>", html)
        self.assertNotIn("removed", html)
        self.assertNotIn("$3.00", html)

    def test_text_is_escaped(self):
        html = render(standing_note="a < b & c")
        self.assertIn("a &lt; b &amp; c", html)
        self.assertNotIn("a < b", html)

    def test_page_basics(self):
        html = render()
        self.assertTrue(html.startswith("<!doctype html>"))
        self.assertIn('<html lang="ja">', html)
        self.assertIn('<meta name="viewport"', html)
        self.assertIn("<title>ShareOJの運用費</title>", html)
        self.assertIn("AWSの請求額・税込", html)
        for banned in ("noindex", "<script", "http://", "${"):
            self.assertNotIn(banned, html)

    def test_output_is_public(self):
        costs.assert_public(render())


if __name__ == "__main__":
    unittest.main()
