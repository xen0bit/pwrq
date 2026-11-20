"""Writes pkg/d1/testdata/parsing_fixtures.json from d1's parsing.py: numbers, units and
dates as the reference reads them.

    python internal/tools/d1fixtures/parsing_fixtures.py D1_SOURCE_DIR OUT
"""
import importlib.util
import json
import sys

NUMBERS = [
    ("56.00 EUR", "", "EUR"), ("12.50", "", None), ("1,250", "", None), ("1,250", "price 3.50", None),
    ("1.250", "Preis 3,50 €", None), ("1.234.567", "", None), ("1 234,50", "", None), ("€1.234,50", "", None),
    ("$1,234.50", "", None), ("(42)", "", None), ("-7", "", None), ("−7", "", None), ("+3", "", None),
    ("15%", "", None), ("3.5 million", "", None), ("2bn", "revenue", None), ("2bn", "", None), ("12k", "$", None),
    ("1,23,45,678", "₹ amounts", None), ("¥5000", "", None), ("1'234.5", "", None), ("12 345", "", None),
    ("abc", "", None), ("", "", None), ("3,14", "lang: fr", None), ("3.14", "", None), ("0.5", "", None),
    ("1e5", "", None), ("100 USD", "", None), ("USD 100", "", None), ("-$5", "", None), ("$-5", "", None),
    ("(−5)", "", None), ("5 Billion", "", None), ("7 thousand", "", None), ("4.5 Mio", "", None),
]
UNITS = ["15%", "€12", "12 EUR", "12 USD and 3 EUR", "12", "Rs. 400", "5 kr"]
DATES = [
    ("3 March 2026", ""), ("March 3, 2026", ""), ("2026-03-03", ""), ("03/04/2026", ""), ("03/04/2026", "date format dd/mm/yyyy"),
    ("03/04/2026", "lang: en"), ("13/04/2026", ""), ("03/04/26", "issued 2026-01-01"), ("03/04/26", ""),
    ("2026年10月20日", ""), ("20 octobre 2026", ""), ("Tue, 20 Oct 2026", ""), ("segunda-feira, 20 de outubro de 2026", ""),
    ("2026-03-03 10:30", ""), ("2026-03-03T10:30:15Z", ""), ("2026-03-03 9:05:01.5+02:00", ""), ("31/02/2026", ""),
    ("1st of May 2026", ""), ("not a date", ""), ("20.10.2026", "Datum tt.mm.jjjj"), ("10/20/2026", "x 13/01/2026"),
    ("5 мая 2026", ""), ("3 März 2026", ""),
]


def main(src, out):
    spec = importlib.util.spec_from_file_location("parsing", f"{src}/parsing.py")
    p = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(p)
    data = {
        "numbers": [{"text": t, "context": c, "unit": u, "value": p.parse_number(t, c, unit=u)} for t, c, u in NUMBERS],
        "units": [{"text": t, "unit": p.written_unit(t)} for t in UNITS],
        "dates": [{"text": t, "context": c, "value": p.parse_date(t, c)[0], "ambiguous": p.parse_date(t, c)[1]}
                  for t, c in DATES],
    }
    with open(out, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=0)


if __name__ == "__main__":
    main(*sys.argv[1:])
