"""Runs the shipped settings against the kinds of charge they must sort."""

import json
from pathlib import Path
import sys
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import costs
import page

CONFIG = json.loads(Path(__file__).resolve().parents[1].joinpath("config.json").read_text(encoding="utf-8"))
RDS = "Amazon Relational Database Service"


def groups_of(lines):
    result = {
        "TimePeriod": {"Start": "2026-12-01", "End": "2027-01-01"},
        "Total": {},
        "Estimated": False,
        "Groups": [
            {"Keys": [service, record_type], "Metrics": {"UnblendedCost": {"Amount": amount, "Unit": "USD"}}}
            for service, record_type, amount in lines
        ],
    }
    return {row["group"]: row["amount"] for row in costs.summarize([result], CONFIG)["2026-12"]["rows"]}


class ShippedConfigTest(unittest.TestCase):
    def test_reserved_instance_upfront_fee_is_a_prepayment(self):
        self.assertEqual(
            groups_of([(RDS, "Fee", "649.00"), (RDS, "Usage", "12.00")]),
            {"prepay": "649.00", "database": "12.00"},
        )

    def test_savings_plan_fees_are_prepayments(self):
        self.assertEqual(
            groups_of([
                ("Savings Plans for AWS Compute usage", "SavingsPlanUpfrontFee", "300.00"),
                ("Savings Plans for AWS Compute usage", "SavingsPlanRecurringFee", "4.00"),
            ]),
            {"prepay": "304.00"},
        )

    def test_tax_and_unknown_services_have_rows(self):
        self.assertEqual(
            groups_of([("Tax", "Tax", "3.10"), ("Amazon Braket", "Usage", "1.00")]),
            {"other": "1.00", "tax": "3.10"},
        )

    def test_every_row_has_a_column_on_the_page(self):
        # One dollar in each row the settings can produce: the page must show all of it, in four columns.
        rows = [{"group": group["id"], "amount": "1.00"} for group in CONFIG["groups"]]
        data = {"version": 1, "updated": "2026-12-08", "months": {"2026-11": {"total": f"{len(rows)}.00", "rows": rows}}}
        html = page.render(data, CONFIG, {})
        cells = [part.split("<")[0] for part in html.split('<td class="expense">')[1:]]
        self.assertEqual(cells, ["$5.00", "$1.00", "$1.00", "$1.00"])
        self.assertIn(f'<td class="balance">-${len(rows)}.00</td>', html)


if __name__ == "__main__":
    unittest.main()
