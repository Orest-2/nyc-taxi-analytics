"""
load_data.py
------------
Downloads NYC Yellow Taxi data (Jan 2025) from the TLC open dataset and
loads it into a local DuckDB database.

Falls back to generating realistic synthetic data if the download fails
(e.g. network restrictions in a sandbox environment).

Usage:
    python scripts/load_data.py
"""

import sys
import pathlib
import urllib.request
import duckdb

DATA_DIR = pathlib.Path(__file__).parent.parent / "data"
DB_PATH  = pathlib.Path(__file__).parent.parent / "taxi.duckdb"
PARQUET  = DATA_DIR / "yellow_2025_01.parquet"

TLC_URL = (
    "https://d37ci6vzurychx.cloudfront.net/trip-data/"
    "yellow_tripdata_2025-01.parquet"
)

DATA_DIR.mkdir(exist_ok=True)


def download_data() -> bool:
    print(f"Downloading from {TLC_URL} ...")
    try:
        urllib.request.urlretrieve(TLC_URL, PARQUET)
        print(f"Saved to {PARQUET}  ({PARQUET.stat().st_size / 1_048_576:.1f} MB)")
        return True
    except Exception as exc:
        print(f"Download failed: {exc}")
        return False


def generate_synthetic(con: duckdb.DuckDBPyConnection) -> None:
    """Generate 100 000 rows of synthetic NYC taxi data matching the real TLC schema."""
    print("Generating 100 000 rows of synthetic NYC taxi data ...")
    con.execute("""
        CREATE OR REPLACE TABLE trips AS
        SELECT
            (id % 2 + 1)::INTEGER                                               AS VendorID,
            TIMESTAMP '2025-01-01 00:00:00'
                + INTERVAL (random() * 31 * 86400) SECOND                       AS tpep_pickup_datetime,
            TIMESTAMP '2025-01-01 00:00:00'
                + INTERVAL (random() * 31 * 86400 + 600 + random()*5400) SECOND AS tpep_dropoff_datetime,
            (1 + (random() * 5)::INT)::BIGINT                                   AS passenger_count,
            round(0.5 + random() * 29.5, 2)                                     AS trip_distance,
            CASE WHEN random()<0.92 THEN 1 ELSE 2 END::BIGINT                   AS RatecodeID,
            CASE WHEN random() < 0.85 THEN 'N' ELSE 'Y' END                    AS store_and_fwd_flag,
            (1 + (id % 263))::INTEGER                                           AS PULocationID,
            (1 + ((id + 37) % 263))::INTEGER                                    AS DOLocationID,
            CASE
                WHEN random() < 0.68 THEN 1
                WHEN random() < 0.90 THEN 2
                WHEN random() < 0.95 THEN 3
                ELSE 4
            END::BIGINT                                                          AS payment_type,
            round(3.0 + random() * 67, 2)                                       AS fare_amount,
            round(CASE WHEN random()<0.3 THEN 0.0 ELSE random()*3 END, 2)      AS extra,
            0.5::DOUBLE                                                          AS mta_tax,
            round(CASE WHEN random()<0.32 THEN 0.0 ELSE random()*8 END, 2)     AS tip_amount,
            round(CASE WHEN random()<0.80 THEN 0.0 ELSE random()*6 END, 2)     AS tolls_amount,
            0.3::DOUBLE                                                          AS improvement_surcharge,
            round(3.0 + random() * 80, 2)                                       AS total_amount,
            round(CASE WHEN random()<0.50 THEN 0.0 ELSE 2.5 END, 2)           AS congestion_surcharge,
            round(CASE WHEN random()<0.90 THEN 0.0 ELSE 1.75 END, 2)          AS Airport_fee,
            round(CASE WHEN random()<0.70 THEN 0.0 ELSE 0.75 END, 2)          AS cbd_congestion_fee
        FROM range(1, 100001) t(id)
    """)
    con.execute(f"COPY trips TO '{PARQUET}' (FORMAT PARQUET)")
    print(f"Parquet written to {PARQUET}")


def load_parquet(con: duckdb.DuckDBPyConnection) -> None:
    con.execute(f"""
        CREATE OR REPLACE TABLE trips AS
        SELECT * FROM read_parquet('{PARQUET}')
    """)


def print_summary(con: duckdb.DuckDBPyConnection) -> None:
    rows = con.execute("SELECT COUNT(*) FROM trips").fetchone()[0]
    cols = len(con.execute("DESCRIBE trips").fetchall())
    size = PARQUET.stat().st_size / 1_048_576
    print(f"\n{'─'*45}")
    print(f"  Rows   : {rows:>12,}")
    print(f"  Columns: {cols:>12}")
    print(f"  Parquet: {size:>11.1f} MB")
    print(f"  DB     : {DB_PATH}")
    print(f"{'─'*45}\n")
    print("Schema:")
    rows = con.execute("DESCRIBE trips").fetchall()
    print(f"  {'column_name':<25} {'column_type'}")
    print(f"  {'-'*25} {'-'*15}")
    for row in rows:
        print(f"  {row[0]:<25} {row[1]}")


def main() -> None:
    print(f"\n=== NYC Taxi Analytics — Data Loader ===\n")

    con = duckdb.connect(str(DB_PATH))

    if not PARQUET.exists():
        if not download_data():
            generate_synthetic(con)
            print_summary(con)
            return

    print("Loading parquet into DuckDB ...")
    load_parquet(con)
    print_summary(con)


if __name__ == "__main__":
    main()
