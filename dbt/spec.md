# dbt Modelling Spec — NYC Taxi Analytics

**Feature:** Model raw TLC trips into a dimensional warehouse with dbt  
**Source:** `trips` table (TLC Yellow Taxi Jan 2025, 3.4M rows, 20 columns)  
**Target:** DuckDB (same `taxi.duckdb` file)  
**dbt adapter:** `dbt-duckdb`

---

## Goals

- Replace raw column references in `analysis.sql` with clean, named dimensions
- Make queries readable without needing to know `VendorID = 1` means "Creative Mobile"
- Lay the foundation for adding more months, more tables, and ML feature stores
- Demonstrate dbt patterns expected in a DE role: sources, staging, dimensions, facts, tests, docs

---

## Source Definition

File: `models/sources.yml`

```yaml
version: 2

sources:
  - name: raw
    schema: main
    description: Raw parquet data loaded directly into DuckDB by load_data.py
    tables:
      - name: trips
        description: TLC Yellow Taxi trip records, Jan 2025
        columns:
          - name: VendorID
            description: "1 = Creative Mobile Technologies, 2 = VeriFone Inc."
            tests: [not_null, accepted_values: { values: [1, 2] }]
          - name: tpep_pickup_datetime
            tests: [not_null]
          - name: tpep_dropoff_datetime
            tests: [not_null]
          - name: PULocationID
            tests: [not_null]
          - name: DOLocationID
            tests: [not_null]
          - name: fare_amount
            tests: [not_null]
          - name: total_amount
            tests: [not_null]
          - name: payment_type
            tests:
              - accepted_values:
                  values: [1, 2, 3, 4]
                  # 1=Credit Card, 2=Cash, 3=No Charge, 4=Dispute
          - name: RatecodeID
            tests:
              - accepted_values:
                  values: [1, 2, 3, 4, 5, 6]
                  # 1=Standard, 2=JFK, 3=Newark, 4=Nassau/Westchester,
                  # 5=Negotiated, 6=Group ride
```

---

## Layer Architecture

```
sources (raw.trips)
    │
    ▼
staging/stg_trips.sql          — rename, cast, derive duration, filter garbage rows
    │
    ├──▶ dim_vendor.sql         — vendor lookup (2 rows)
    ├──▶ dim_payment_type.sql   — payment method lookup (4 rows)
    ├──▶ dim_rate_code.sql      — rate code lookup (6 rows)
    └──▶ dim_zone.sql           — TLC taxi zone lookup (265 rows, seeded from CSV)
    │
    ▼
fact_trips.sql                 — one row per trip, FK references to all dims
    │
    ▼
mart_daily_summary.sql         — pre-aggregated daily rollup for dashboards
```

---

## Model Specs

### `staging/stg_trips.sql`

**Purpose:** Single entry point for the raw source. Rename columns to snake_case,
cast types, derive `trip_duration_sec`, and filter out known-bad rows.  
**Materialization:** `view` (no storage cost; always fresh)

```sql
with source as (
    select * from {{ source('raw', 'trips') }}
),

cleaned as (
    select
        -- identifiers
        VendorID                                    as vendor_id,
        PULocationID                                as pickup_zone_id,
        DOLocationID                                as dropoff_zone_id,
        RatecodeID                                  as rate_code_id,
        payment_type                                as payment_type_id,

        -- timestamps
        tpep_pickup_datetime                        as pickup_at,
        tpep_dropoff_datetime                       as dropoff_at,
        epoch(
            tpep_dropoff_datetime - tpep_pickup_datetime
        )                                           as trip_duration_sec,

        -- trip facts
        passenger_count,
        trip_distance,
        store_and_fwd_flag,

        -- financials
        fare_amount,
        extra,
        mta_tax,
        tip_amount,
        tolls_amount,
        improvement_surcharge,
        congestion_surcharge,
        Airport_fee                                 as airport_fee,
        cbd_congestion_fee,
        total_amount

    from source

    where
        -- drop rows with impossible timestamps
        tpep_dropoff_datetime > tpep_pickup_datetime
        -- drop negative fares (refunds, data errors)
        and fare_amount >= 0
        -- drop trips with impossible distances
        and trip_distance >= 0
        -- drop zero-duration trips with non-zero fare (meter errors)
        and not (
            epoch(tpep_dropoff_datetime - tpep_pickup_datetime) = 0
            and fare_amount > 0
        )
)

select * from cleaned
```

**Tests (`staging/schema.yml`):**

```yaml
models:
  - name: stg_trips
    columns:
      - name: pickup_at
        tests: [not_null]
      - name: dropoff_at
        tests: [not_null]
      - name: trip_duration_sec
        tests:
          - not_null
          - dbt_utils.expression_is_true:
              expression: ">= 0"
      - name: fare_amount
        tests:
          - dbt_utils.expression_is_true:
              expression: ">= 0"
      - name: total_amount
        tests:
          - dbt_utils.expression_is_true:
              expression: ">= 0"
```

---

### `dim_vendor.sql`

**Purpose:** Human-readable vendor lookup. Source of truth for vendor names —
never hardcode `CASE VendorID WHEN 1 THEN ...` in queries again.  
**Materialization:** `table` (tiny; rarely changes)  
**Row count:** 2

```sql
select
    vendor_id,
    case vendor_id
        when 1 then 'Creative Mobile Technologies'
        when 2 then 'VeriFone Inc.'
        else        'Unknown (' || vendor_id || ')'
    end                             as vendor_name,
    case vendor_id
        when 1 then 'CMT'
        when 2 then 'VTS'
        else        'UNK'
    end                             as vendor_abbr

from (
    select distinct vendor_id
    from {{ ref('stg_trips') }}
)

order by vendor_id
```

**Tests:**

```yaml
- name: dim_vendor
  columns:
    - name: vendor_id
      tests: [unique, not_null]
    - name: vendor_name
      tests: [not_null]
```

---

### `dim_payment_type.sql`

**Purpose:** Payment method lookup with labels and a boolean convenience flag.  
**Materialization:** `table`  
**Row count:** 4

```sql
select
    payment_type_id,
    case payment_type_id
        when 1 then 'Credit Card'
        when 2 then 'Cash'
        when 3 then 'No Charge'
        when 4 then 'Dispute'
        else        'Unknown'
    end                             as payment_label,
    -- convenience: tips are only recorded for card payments
    payment_type_id = 1             as records_tip

from (
    select distinct payment_type_id
    from {{ ref('stg_trips') }}
)

order by payment_type_id
```

**Tests:**

```yaml
- name: dim_payment_type
  columns:
    - name: payment_type_id
      tests: [unique, not_null]
```

---

### `dim_rate_code.sql`

**Purpose:** TLC rate code lookup (standard, JFK flat rate, Newark, etc.)  
**Materialization:** `table`  
**Row count:** up to 6

```sql
select
    rate_code_id,
    case rate_code_id
        when 1 then 'Standard Rate'
        when 2 then 'JFK'
        when 3 then 'Newark'
        when 4 then 'Nassau or Westchester'
        when 5 then 'Negotiated Fare'
        when 6 then 'Group Ride'
        else        'Unknown'
    end                             as rate_description,
    -- flat-rate routes have predictable fares; useful for anomaly detection
    rate_code_id in (2, 3, 4)       as is_flat_rate

from (
    select distinct rate_code_id
    from {{ ref('stg_trips') }}
    where rate_code_id is not null
)

order by rate_code_id
```

---

### `dim_zone.sql` — seeded from CSV

**Purpose:** TLC taxi zone lookup (zone name, borough, service zone).  
**Why seed:** Zone names don't exist in the trip data — they come from the
[TLC Zone lookup CSV](https://d37ci6vzurychx.cloudfront.net/misc/taxi_zone_lookup.csv).  
**Materialization:** `table` (built from seed, not from trips)  
**Row count:** 265 (263 zones + EWR + Unknown)

**Seed file:** `seeds/taxi_zone_lookup.csv`

```
LocationID,Borough,Zone,service_zone
1,EWR,Newark Airport,EWR
2,Queens,Jamaica Bay,Boro Zone
3,Bronx,Allerton/Pelham Gardens,Boro Zone
...
132,Queens,JFK Airport,Airports
138,Queens,LaGuardia Airport,Airports
...
```

**Model:** `models/dimensions/dim_zone.sql`

```sql
select
    "LocationID"                    as zone_id,
    "Borough"                       as borough,
    "Zone"                          as zone_name,
    "service_zone"                  as service_zone,

    -- convenience flags used in fact_trips downstream
    "service_zone" = 'Airports'     as is_airport,
    "Borough" = 'Manhattan'         as is_manhattan

from {{ ref('taxi_zone_lookup') }}
```

**Tests:**

```yaml
- name: dim_zone
  columns:
    - name: zone_id
      tests: [unique, not_null]
    - name: borough
      tests:
        - accepted_values:
            values:
              [Bronx, Brooklyn, EWR, Manhattan, Queens, Staten Island, Unknown]
    - name: zone_name
      tests: [not_null]
```

---

### `fact_trips.sql`

**Purpose:** Central fact table. One row per trip. All FK references resolve to
dimension keys. Derived metrics pre-computed so queries are simple.  
**Materialization:** `table` (3.4M rows — worth materialising)  
**Partitioning hint:** In production on Snowflake/BigQuery, partition by `pickup_date`.

```sql
with trips as (
    select * from {{ ref('stg_trips') }}
),

zones as (
    select zone_id, borough, zone_name, is_airport, is_manhattan
    from {{ ref('dim_zone') }}
)

select
    -- surrogate key
    {{ dbt_utils.generate_surrogate_key([
        'pickup_at',
        'dropoff_at',
        'vendor_id',
        'pickup_zone_id'
    ]) }}                                       as trip_sk,

    -- foreign keys
    t.vendor_id,
    t.pickup_zone_id,
    t.dropoff_zone_id,
    t.rate_code_id,
    t.payment_type_id,

    -- degenerate dimensions (low cardinality, no separate dim needed)
    t.store_and_fwd_flag,
    t.passenger_count,

    -- date/time grains
    t.pickup_at,
    t.dropoff_at,
    t.pickup_at::date                           as pickup_date,
    date_part('hour', t.pickup_at)              as pickup_hour,
    date_part('dow',  t.pickup_at)              as pickup_dow,  -- 0=Sun

    -- measures
    t.trip_duration_sec,
    t.trip_distance,

    -- financials (raw columns kept for auditability)
    t.fare_amount,
    t.extra,
    t.mta_tax,
    t.tip_amount,
    t.tolls_amount,
    t.improvement_surcharge,
    t.congestion_surcharge,
    t.airport_fee,
    t.cbd_congestion_fee,
    t.total_amount,

    -- derived financial metrics
    t.total_amount - t.fare_amount              as total_surcharges,
    case
        when t.trip_distance > 0
        then round(t.fare_amount / t.trip_distance, 2)
    end                                         as fare_per_mile,
    case
        when t.trip_duration_sec > 0
        then round(t.fare_amount / (t.trip_duration_sec / 60.0), 2)
    end                                         as fare_per_minute,
    t.tip_amount > 0                            as has_tip,

    -- zone flags (denormalised for query speed)
    pu.is_airport                               as pickup_is_airport,
    pu.is_manhattan                             as pickup_is_manhattan,
    do_.is_airport                              as dropoff_is_airport,
    do_.is_manhattan                            as dropoff_is_manhattan

from trips t
left join zones pu  on t.pickup_zone_id  = pu.zone_id
left join zones do_ on t.dropoff_zone_id = do_.zone_id
```

**Tests:**

```yaml
- name: fact_trips
  columns:
    - name: trip_sk
      tests: [unique, not_null]
    - name: vendor_id
      tests:
        - not_null
        - relationships:
            to: ref('dim_vendor')
            field: vendor_id
    - name: pickup_zone_id
      tests:
        - relationships:
            to: ref('dim_zone')
            field: zone_id
    - name: dropoff_zone_id
      tests:
        - relationships:
            to: ref('dim_zone')
            field: zone_id
    - name: payment_type_id
      tests:
        - relationships:
            to: ref('dim_payment_type')
            field: payment_type_id
    - name: fare_amount
      tests:
        - dbt_utils.expression_is_true:
            expression: ">= 0"
    - name: trip_duration_sec
      tests:
        - dbt_utils.expression_is_true:
            expression: ">= 0"
    - name: total_surcharges
      tests:
        - dbt_utils.expression_is_true:
            expression: ">= 0"
```

---

### `mart_daily_summary.sql`

**Purpose:** Pre-aggregated daily rollup. Feeds dashboards directly — no window
functions at query time.  
**Materialization:** `table`

```sql
select
    pickup_date,

    -- volume
    count(*)                                    as total_trips,
    sum(passenger_count)                        as total_passengers,

    -- distance & duration
    round(avg(trip_distance), 2)                as avg_distance_mi,
    round(avg(trip_duration_sec) / 60.0, 1)     as avg_duration_min,

    -- financials
    round(sum(fare_amount), 2)                  as total_fare,
    round(sum(tip_amount), 2)                   as total_tips,
    round(sum(total_amount), 2)                 as total_collected,
    round(avg(fare_amount), 2)                  as avg_fare,
    round(avg(tip_amount), 2)                   as avg_tip,
    round(avg(total_surcharges), 2)             as avg_surcharges,

    -- ratios
    round(
        100.0 * sum(tip_amount) / nullif(sum(fare_amount), 0)
    , 2)                                        as tip_rate_pct,

    -- segment counts
    count(*) filter (where has_tip)             as trips_with_tip,
    count(*) filter (where pickup_is_airport)   as airport_pickups,
    count(*) filter (where pickup_is_manhattan) as manhattan_pickups

from {{ ref('fact_trips') }}

group by pickup_date
order by pickup_date
```

---

## Project File Layout

```
dbt/
├── dbt_project.yml
├── packages.yml
├── profiles.yml
├── seeds/
│   └── taxi_zone_lookup.csv
└── models/
    ├── sources.yml
    ├── staging/
    │   ├── stg_trips.sql
    │   └── schema.yml
    ├── dimensions/
    │   ├── dim_vendor.sql
    │   ├── dim_payment_type.sql
    │   ├── dim_rate_code.sql
    │   ├── dim_zone.sql
    │   └── schema.yml
    ├── facts/
    │   ├── fact_trips.sql
    │   └── schema.yml
    └── marts/
        ├── mart_daily_summary.sql
        └── schema.yml
```

---

## Config Files

**`dbt_project.yml`**

```yaml
name: nyc_taxi
version: "1.0.0"
profile: nyc_taxi

model-paths: ["models"]
seed-paths: ["seeds"]
test-paths: ["tests"]

models:
  nyc_taxi:
    staging:
      +materialized: view
    dimensions:
      +materialized: table
    facts:
      +materialized: table
    marts:
      +materialized: table
```

**`packages.yml`**

```yaml
packages:
  - package: dbt-labs/dbt_utils
    version: [">=1.0.0", "<2.0.0"]
```

**`profiles.yml`**

```yaml
nyc_taxi:
  target: dev
  outputs:
    dev:
      type: duckdb
      path: ../taxi.duckdb
      threads: 4
```

---

## Build Commands

```bash
cd dbt

pip install dbt-duckdb
dbt deps          # install dbt_utils

dbt seed          # load taxi_zone_lookup.csv → dim_zone source
dbt run           # build all models in DAG order
dbt test          # run all schema + relationship tests
dbt build         # seed + run + test in one shot

# docs with full lineage graph
dbt docs generate
dbt docs serve
```

---

## DAG (Lineage)

```
raw.trips (source)
    │
    ▼
stg_trips (view)
    │
    ├──────────────────┬──────────────────┬──────────────────────────┐
    ▼                  ▼                  ▼                          ▼
dim_vendor      dim_payment_type    dim_rate_code         seeds/taxi_zone_lookup
(table)         (table)             (table)                         │
                                                                     ▼
                                                                 dim_zone
                                                                 (table)
    │                  │                  │                          │
    └──────────────────┴──────────────────┴──────────────────────────┘
                                │
                                ▼
                          fact_trips (table)
                                │
                                ▼
                      mart_daily_summary (table)
```

---

## How `analysis.sql` Queries Improve After This

**Before — Q06 Vendor comparison:**

```sql
SELECT
    CASE VendorID
        WHEN 1 THEN 'Creative Mobile (1)'
        WHEN 2 THEN 'VeriFone (2)'
        ELSE        'Other'
    END AS vendor,
    COUNT(*), ROUND(AVG(fare_amount), 2)
FROM trips
GROUP BY VendorID
```

**After:**

```sql
SELECT
    v.vendor_name,
    COUNT(*)                        AS total_trips,
    ROUND(AVG(f.fare_amount), 2)    AS avg_fare
FROM fact_trips f
JOIN dim_vendor v USING (vendor_id)
GROUP BY v.vendor_name
ORDER BY v.vendor_name
```

**Before — Q05 Busiest pickup zones:**

```sql
SELECT PULocationID, COUNT(*) AS pickups
FROM trips
GROUP BY PULocationID
ORDER BY pickups DESC
LIMIT 10
-- reader has to look up what zone 132 is
```

**After:**

```sql
SELECT
    z.zone_name,
    z.borough,
    COUNT(*)                        AS pickups,
    ROUND(AVG(f.fare_amount), 2)    AS avg_fare
FROM fact_trips f
JOIN dim_zone z ON f.pickup_zone_id = z.zone_id
GROUP BY z.zone_name, z.borough
ORDER BY pickups DESC
LIMIT 10
```

---

## Acceptance Criteria

- [ ] `dbt build` exits 0 with no test failures
- [ ] `fact_trips` row count ≤ `stg_trips` row count (filtering in staging is expected; no fan-out)
- [ ] `trip_sk` passes `unique` and `not_null` tests
- [ ] All `relationships` tests pass on `fact_trips` (vendor, zone, payment_type FKs)
- [ ] `mart_daily_summary` has exactly 31 rows (one per day in January)
- [ ] `dim_zone` has 265 rows after `dbt seed`
- [ ] `dbt docs generate` produces a complete lineage DAG
- [ ] Rewritten Q05 and Q06 against the new models return equivalent results to the originals
