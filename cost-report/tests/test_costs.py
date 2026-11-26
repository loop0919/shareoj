import datetime
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import costs

RDS = "Amazon Relational Database Service"
EC2 = "Amazon Elastic Compute Cloud - Compute"
EBS = "EC2 - Other"
PLANS = "Savings Plans for AWS Compute usage"

CONFIG = {
    "start": "2026-09",
    "groups": [
        {"id": "prepay", "label": "サーバーの前払い", "record_types": ["SavingsPlanUpfrontFee", "RIUpfrontExample"]},
        {"id": "database", "label": "データベース", "services": [RDS]},
        {"id": "judge", "label": "ジャッジサーバー", "services": [EC2, EBS]},
        {"id": "other", "label": "その他"},
        {"id": "tax", "label": "消費税", "record_types": ["Tax"]},
    ],
}


def month(start, groups, estimated=False):
    first = datetime.date.fromisoformat(start)
    end = datetime.date(first.year + first.month // 12, first.month % 12 + 1, 1)
    return {
        "TimePeriod": {"Start": start, "End": end.isoformat()},
        "Total": {},
        "Estimated": estimated,
        "Groups": [
            {"Keys": [service, record_type], "Metrics": {"UnblendedCost": {"Amount": amount, "Unit": "USD"}}}
            for service, record_type, amount in groups
        ],
    }


def rows(summary, month_key):
    return {row["group"]: row["amount"] for row in summary[month_key]["rows"]}


class SummarizeTest(unittest.TestCase):
    def test_groups_by_service_and_record_type(self):
        summary = costs.summarize([month("2026-10-01", [
            (RDS, "Usage", "28.404"),
            (EC2, "Usage", "20.10"),
            (EBS, "Usage", "4.00"),
            (PLANS, "SavingsPlanUpfrontFee", "318.00"),
            ("Tax", "Tax", "37.05"),
        ])], CONFIG)
        self.assertEqual(summary, {"2026-10": {"total": "407.55", "rows": [
            {"group": "prepay", "amount": "318.00"},
            {"group": "database", "amount": "28.40"},
            {"group": "judge", "amount": "24.10"},
            {"group": "tax", "amount": "37.05"},
        ]}})

    def test_reserved_upfront_inside_rds_moves_to_prepay(self):
        summary = costs.summarize([month("2026-10-01", [
            (RDS, "RIUpfrontExample", "649.00"),
            (RDS, "Usage", "10.00"),
        ])], CONFIG)
        self.assertEqual(rows(summary, "2026-10"), {"prepay": "649.00", "database": "10.00"})

    def test_unknown_service_goes_to_other(self):
        summary = costs.summarize([month("2026-10-01", [("Amazon Braket", "Usage", "1.50")])], CONFIG)
        self.assertEqual(rows(summary, "2026-10"), {"other": "1.50"})

    def test_rows_sum_to_total(self):
        summary = costs.summarize([month("2026-10-01", [
            (RDS, "Usage", "0.335"),
            (EC2, "Usage", "0.335"),
            ("Amazon Braket", "Usage", "0.335"),
        ])], CONFIG)
        self.assertEqual(summary["2026-10"]["total"], "1.01")
        self.assertEqual(summary["2026-10"]["rows"], [
            {"group": "database", "amount": "0.34"},
            {"group": "judge", "amount": "0.34"},
            {"group": "other", "amount": "0.33"},
        ])

    def test_separately_billed_rows_round_on_their_own(self):
        # AWS invoices an upfront fee and tax as their own lines, so those rows must match the invoice to the cent.
        summary = costs.summarize([month("2026-10-01", [
            (PLANS, "SavingsPlanUpfrontFee", "120.036"),
            (RDS, "Usage", "20.4875"),
            (EC2, "Usage", "30.6564"),
            ("Amazon Braket", "Usage", "7.9594"),
            ("Tax", "Tax", "17.91"),
        ])], CONFIG)
        self.assertEqual(summary["2026-10"], {"total": "197.05", "rows": [
            {"group": "prepay", "amount": "120.04"},
            {"group": "database", "amount": "20.49"},
            {"group": "judge", "amount": "30.65"},
            {"group": "other", "amount": "7.96"},
            {"group": "tax", "amount": "17.91"},
        ]})

    def test_usage_rows_give_back_a_cent_when_fees_round_up(self):
        summary = costs.summarize([month("2026-10-01", [
            (PLANS, "SavingsPlanUpfrontFee", "10.005"),
            (RDS, "Usage", "1.002"),
            ("Tax", "Tax", "5.005"),
        ])], CONFIG)
        self.assertEqual(summary["2026-10"], {"total": "16.01", "rows": [
            {"group": "prepay", "amount": "10.01"},
            {"group": "database", "amount": "0.99"},
            {"group": "tax", "amount": "5.01"},
        ]})

    def test_largest_fee_absorbs_the_cent_without_usage_rows(self):
        summary = costs.summarize([month("2026-10-01", [
            (PLANS, "SavingsPlanUpfrontFee", "10.005"),
            ("Tax", "Tax", "5.005"),
        ])], CONFIG)
        self.assertEqual(summary["2026-10"], {"total": "15.01", "rows": [
            {"group": "prepay", "amount": "10.00"},
            {"group": "tax", "amount": "5.01"},
        ]})

    def test_tiny_amounts_are_dropped(self):
        summary = costs.summarize([month("2026-10-01", [
            (RDS, "Usage", "0.004"),
            (EC2, "Usage", "-0.004"),
            ("Amazon Braket", "Usage", "5.00"),
        ])], CONFIG)
        self.assertEqual(summary["2026-10"], {"total": "5.00", "rows": [{"group": "other", "amount": "5.00"}]})

    def test_credit_makes_row_and_total_negative(self):
        summary = costs.summarize([month("2026-10-01", [
            (RDS, "Credit", "-30.00"),
            (RDS, "Usage", "10.00"),
        ])], CONFIG)
        self.assertEqual(summary["2026-10"], {"total": "-20.00", "rows": [{"group": "database", "amount": "-20.00"}]})

    def test_same_month_on_two_pages_is_added(self):
        summary = costs.summarize([
            month("2026-10-01", [(RDS, "Usage", "10.00")]),
            month("2026-10-01", [(RDS, "Usage", "5.00"), (EC2, "Usage", "1.00")]),
        ], CONFIG)
        self.assertEqual(summary["2026-10"], {"total": "16.00", "rows": [
            {"group": "database", "amount": "15.00"},
            {"group": "judge", "amount": "1.00"},
        ]})

    def test_month_without_groups(self):
        summary = costs.summarize([month("2026-10-01", [])], CONFIG)
        self.assertEqual(summary, {"2026-10": {"total": "0.00", "rows": []}})

    def test_estimated_month_raises_pending(self):
        with self.assertRaises(costs.Pending) as caught:
            costs.summarize([
                month("2026-10-01", [(RDS, "Usage", "10.00")]),
                month("2026-11-01", [(RDS, "Usage", "3.00")], estimated=True),
            ], CONFIG)
        self.assertEqual(caught.exception.months, ["2026-11"])


class QueryPeriodTest(unittest.TestCase):
    def test_from_start_to_first_day_of_this_month(self):
        self.assertEqual(costs.query_period("2026-09", datetime.date(2026, 12, 8)), ("2026-09-01", "2026-12-01"))

    def test_january_ends_the_previous_year(self):
        self.assertEqual(costs.query_period("2026-09", datetime.date(2027, 1, 8)), ("2026-09-01", "2027-01-01"))

    def test_start_is_kept_within_twelve_months(self):
        self.assertEqual(costs.query_period("2026-09", datetime.date(2028, 3, 8)), ("2027-03-01", "2028-03-01"))

    def test_no_finished_month_yet(self):
        self.assertIsNone(costs.query_period("2026-09", datetime.date(2026, 9, 20)))


SEPTEMBER = {"total": "64.10", "rows": [{"group": "database", "amount": "64.10"}]}
OCTOBER = {"total": "412.35", "rows": [{"group": "prepay", "amount": "318.00"}, {"group": "database", "amount": "94.35"}]}
NOVEMBER = {"total": "70.00", "rows": [{"group": "database", "amount": "70.00"}]}


class MergeTest(unittest.TestCase):
    def test_merge_overwrites_and_keeps_old_months(self):
        stored = {"version": 1, "updated": "2026-11-08", "months": {"2026-09": SEPTEMBER, "2026-10": OCTOBER}}
        corrected = {"total": "400.00", "rows": [{"group": "database", "amount": "400.00"}]}
        merged, changed = costs.merge(stored, {"2026-10": corrected, "2026-11": NOVEMBER}, datetime.date(2026, 12, 8))
        self.assertTrue(changed)
        self.assertEqual(merged, {"version": 1, "updated": "2026-12-08", "months": {
            "2026-09": SEPTEMBER, "2026-10": corrected, "2026-11": NOVEMBER,
        }})

    def test_merge_without_change_keeps_updated(self):
        stored = {"version": 1, "updated": "2026-11-08", "months": {"2026-09": SEPTEMBER, "2026-10": OCTOBER}}
        merged, changed = costs.merge(stored, {"2026-10": OCTOBER}, datetime.date(2026, 12, 8))
        self.assertFalse(changed)
        self.assertEqual(merged, stored)

    def test_merge_into_empty(self):
        stored = {"version": 1, "updated": None, "months": {}}
        merged, changed = costs.merge(stored, {"2026-09": SEPTEMBER}, datetime.date(2026, 10, 8))
        self.assertTrue(changed)
        self.assertEqual(merged, {"version": 1, "updated": "2026-10-08", "months": {"2026-09": SEPTEMBER}})

    def test_merge_does_not_edit_the_stored_copy(self):
        stored = {"version": 1, "updated": None, "months": {}}
        costs.merge(stored, {"2026-09": SEPTEMBER}, datetime.date(2026, 10, 8))
        self.assertEqual(stored, {"version": 1, "updated": None, "months": {}})


class AssertPublicTest(unittest.TestCase):
    def test_rejects_role_arn(self):
        with self.assertRaises(ValueError):
            costs.assert_public("arn:aws:iam::123456789012:role/x")

    def test_rejects_bare_account_id(self):
        with self.assertRaises(ValueError):
            costs.assert_public("id 123456789012")

    def test_accepts_amounts_and_dates(self):
        costs.assert_public("$1,234,567.89 2026-12-08")


if __name__ == "__main__":
    unittest.main()
