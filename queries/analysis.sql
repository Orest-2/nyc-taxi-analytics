-- =============================================================
-- NYC Yellow Taxi 2025 — Analytical Query Suite
-- Engine : DuckDB  |  Dataset: TLC Yellow Taxi Jan 2025
-- Schema : Real TLC parquet (3.4M rows, 20 columns)
-- =============================================================
-- Column reference:
--   VendorID                  1=Creative Mobile, 2=VeriFone
--   tpep_pickup_datetime      pickup timestamp
--   tpep_dropoff_datetime     dropoff timestamp
--   PULocationID / DOLocationID  TLC taxi zone IDs (1-263)
--   payment_type              1=Credit Card 2=Cash 3=No Charge 4=Dispute
--   fare_amount               metered fare
--   total_amount              fare + all surcharges + tip
--   congestion_surcharge      NYC CBD congestion fee ($2.50)
--   Airport_fee               JFK/LGA surcharge ($1.75)
--   cbd_congestion_fee        post-2025 congestion pricing fee
-- =============================================================


-- ── Q01 ── Daily trip volume & revenue trend ─────────────────
-- Answers: How does demand and revenue vary day-by-day in January?
-- Technique: DATE_TRUNC, window function for 7-day rolling avg

SELECT
    DATE_TRUNC('day', tpep_pickup_datetime)::DATE  AS trip_date,
    COUNT(*)                                        AS total_trips,
    ROUND(SUM(fare_amount), 2)                      AS total_fare,
    ROUND(AVG(fare_amount), 2)                      AS avg_fare,
    ROUND(SUM(total_amount), 2)                     AS total_collected,
    ROUND(
        AVG(COUNT(*)) OVER (
            ORDER BY DATE_TRUNC('day', tpep_pickup_datetime)
            ROWS BETWEEN 6 PRECEDING AND CURRENT ROW
        ), 1
    )                                               AS rolling_7d_avg_trips
FROM trips
GROUP BY DATE_TRUNC('day', tpep_pickup_datetime)
ORDER BY trip_date;


-- ── Q02 ── Peak hour demand heatmap ──────────────────────────
-- Answers: Which hours of which weekdays are busiest?
-- Technique: EXTRACT, pivot-style aggregation

SELECT
    EXTRACT(DOW  FROM tpep_pickup_datetime)::INT  AS day_of_week,  -- 0=Sun
    DAYNAME(tpep_pickup_datetime)                 AS day_name,
    EXTRACT(HOUR FROM tpep_pickup_datetime)::INT  AS hour_of_day,
    COUNT(*)                                      AS trip_count,
    ROUND(AVG(fare_amount), 2)                    AS avg_fare,
    ROUND(AVG(total_amount), 2)                   AS avg_total
FROM trips
GROUP BY 1, 2, 3
ORDER BY trip_count DESC
LIMIT 20;


-- ── Q03 ── Tip behaviour by payment type ─────────────────────
-- Answers: Do card payers tip more than cash payers?
-- Technique: CASE/DECODE, ratio calculation, FILTER aggregate

SELECT
    CASE payment_type
        WHEN 1 THEN 'Credit Card'
        WHEN 2 THEN 'Cash'
        WHEN 3 THEN 'No Charge'
        WHEN 4 THEN 'Dispute'
        ELSE        'Unknown'
    END                                                     AS payment_label,
    COUNT(*)                                                AS trip_count,
    ROUND(AVG(tip_amount), 2)                               AS avg_tip,
    ROUND(AVG(fare_amount), 2)                              AS avg_fare,
    ROUND(
        100.0 * AVG(tip_amount) / NULLIF(AVG(fare_amount), 0)
    , 2)                                                    AS tip_pct_of_fare,
    COUNT(*) FILTER (WHERE tip_amount > 0)                  AS trips_with_tip,
    ROUND(
        100.0 * COUNT(*) FILTER (WHERE tip_amount > 0)
              / COUNT(*)
    , 1)                                                    AS pct_tipped
FROM trips
GROUP BY payment_type
ORDER BY trip_count DESC;


-- ── Q04 ── Fare-per-mile efficiency by distance bucket ───────
-- Answers: Are short or long trips more economically efficient per mile?
-- Technique: CASE bucketing, ratio, derived duration from timestamps

SELECT
    CASE
        WHEN trip_distance <  1  THEN '1_under_1mi'
        WHEN trip_distance <  3  THEN '2_1to3mi'
        WHEN trip_distance <  7  THEN '3_3to7mi'
        WHEN trip_distance < 15  THEN '4_7to15mi'
        ELSE                          '5_over15mi'
    END                                                         AS distance_bucket,
    COUNT(*)                                                    AS trips,
    ROUND(AVG(trip_distance), 2)                                AS avg_distance_mi,
    ROUND(AVG(fare_amount),   2)                                AS avg_fare,
    ROUND(AVG(fare_amount / NULLIF(trip_distance, 0)), 2)       AS fare_per_mile,
    ROUND(AVG(
        EPOCH(tpep_dropoff_datetime - tpep_pickup_datetime) / 60.0
    ), 1)                                                       AS avg_duration_min
FROM trips
WHERE trip_distance > 0
  AND tpep_dropoff_datetime > tpep_pickup_datetime
GROUP BY distance_bucket
ORDER BY distance_bucket;


-- ── Q05 ── Top 10 busiest pickup zones ───────────────────────
-- Answers: Which TLC zones generate the most pickups?
-- Technique: Simple aggregation, rank by volume

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


-- ── Q06 ── Vendor performance comparison ─────────────────────
-- Answers: How do the two vendors differ on key operational metrics?
-- Technique: Multi-metric aggregation, PERCENTILE_CONT

SELECT
    v.vendor_name,
    COUNT(*)                        AS total_trips,
    ROUND(AVG(f.fare_amount), 2)    AS avg_fare
FROM fact_trips f
JOIN dim_vendor v USING (vendor_id)
GROUP BY v.vendor_name
ORDER BY v.vendor_name


-- ── Q07 ── Revenue cohort: top 1% vs rest ────────────────────
-- Answers: How much of total revenue do the highest-fare trips contribute?
-- Technique: PERCENT_RANK window function, CTE

WITH ranked AS (
    SELECT
        fare_amount,
        tip_amount,
        total_amount,
        PERCENT_RANK() OVER (ORDER BY fare_amount) AS fare_pct_rank
    FROM trips
    WHERE fare_amount > 0
),
cohorts AS (
    SELECT
        CASE
            WHEN fare_pct_rank >= 0.99 THEN 'Top 1%'
            WHEN fare_pct_rank >= 0.90 THEN 'Top 10%'
            WHEN fare_pct_rank >= 0.75 THEN 'Top 25%'
            ELSE                            'Bottom 75%'
        END         AS cohort,
        total_amount
    FROM ranked
)
SELECT
    cohort,
    COUNT(*)                                           AS trips,
    ROUND(SUM(total_amount), 2)                        AS total_revenue,
    ROUND(AVG(total_amount), 2)                        AS avg_revenue,
    ROUND(
        100.0 * SUM(total_amount)
              / SUM(SUM(total_amount)) OVER ()
    , 2)                                               AS revenue_share_pct
FROM cohorts
GROUP BY cohort
ORDER BY avg_revenue DESC;


-- ── Q08 ── Fare anomaly detection ────────────────────────────
-- Answers: Which trips have suspiciously high or low fares?
-- Technique: Z-score using STDDEV, CROSS JOIN stats CTE

WITH stats AS (
    SELECT
        AVG(fare_amount)    AS mean_fare,
        STDDEV(fare_amount) AS std_fare
    FROM trips
    WHERE fare_amount > 0
),
scored AS (
    SELECT
        t.tpep_pickup_datetime,
        t.fare_amount,
        t.trip_distance,
        t.PULocationID,
        t.DOLocationID,
        t.payment_type,
        ROUND(
            (t.fare_amount - s.mean_fare) / NULLIF(s.std_fare, 0)
        , 2)                AS fare_z_score
    FROM trips t CROSS JOIN stats s
    WHERE t.fare_amount > 0
)
SELECT
    tpep_pickup_datetime,
    fare_amount,
    trip_distance,
    PULocationID,
    DOLocationID,
    fare_z_score,
    CASE
        WHEN fare_z_score >  3 THEN 'High outlier'
        WHEN fare_z_score < -2 THEN 'Low outlier'
        ELSE 'Normal'
    END AS anomaly_flag
FROM scored
WHERE ABS(fare_z_score) > 2
ORDER BY fare_z_score DESC
LIMIT 20;


-- ── Q09 ── Running cumulative revenue by day ──────────────────
-- Answers: What does cumulative January revenue look like?
-- Technique: Running SUM with UNBOUNDED PRECEDING window frame

SELECT
    DATE_TRUNC('day', tpep_pickup_datetime)::DATE   AS trip_date,
    COUNT(*)                                         AS daily_trips,
    ROUND(SUM(total_amount), 2)                      AS daily_revenue,
    ROUND(
        SUM(SUM(total_amount)) OVER (
            ORDER BY DATE_TRUNC('day', tpep_pickup_datetime)
            ROWS BETWEEN UNBOUNDED PRECEDING AND CURRENT ROW
        ), 2
    )                                                AS cumulative_revenue
FROM trips
GROUP BY DATE_TRUNC('day', tpep_pickup_datetime)
ORDER BY trip_date;


-- ── Q10 ── Day-over-day trip count change ─────────────────────
-- Answers: Is demand growing, shrinking, or flat through the month?
-- Technique: LAG window function, percentage delta

WITH daily AS (
    SELECT
        DATE_TRUNC('day', tpep_pickup_datetime)::DATE AS trip_date,
        COUNT(*)                                       AS trips
    FROM trips
    GROUP BY 1
)
SELECT
    trip_date,
    trips,
    LAG(trips) OVER (ORDER BY trip_date)              AS prev_day_trips,
    trips - LAG(trips) OVER (ORDER BY trip_date)      AS delta,
    ROUND(
        100.0 * (trips - LAG(trips) OVER (ORDER BY trip_date))
              / NULLIF(LAG(trips) OVER (ORDER BY trip_date), 0)
    , 2)                                              AS pct_change
FROM daily
ORDER BY trip_date;


-- ── Q11 ── Surcharge revenue breakdown ───────────────────────
-- Answers: How much revenue comes from each surcharge type?
-- Technique: Multi-column SUM, percentage of total, real TLC columns

SELECT
    'fare_amount'          AS component,
    ROUND(SUM(fare_amount), 2)          AS total,
    ROUND(100.0 * SUM(fare_amount)          / SUM(total_amount), 2) AS pct_of_total
FROM trips WHERE total_amount > 0
UNION ALL
SELECT 'tip_amount',
    ROUND(SUM(tip_amount), 2),
    ROUND(100.0 * SUM(tip_amount)           / SUM(total_amount), 2)
FROM trips WHERE total_amount > 0
UNION ALL
SELECT 'tolls_amount',
    ROUND(SUM(tolls_amount), 2),
    ROUND(100.0 * SUM(tolls_amount)         / SUM(total_amount), 2)
FROM trips WHERE total_amount > 0
UNION ALL
SELECT 'congestion_surcharge',
    ROUND(SUM(congestion_surcharge), 2),
    ROUND(100.0 * SUM(congestion_surcharge) / SUM(total_amount), 2)
FROM trips WHERE total_amount > 0
UNION ALL
SELECT 'Airport_fee',
    ROUND(SUM(Airport_fee), 2),
    ROUND(100.0 * SUM(Airport_fee)          / SUM(total_amount), 2)
FROM trips WHERE total_amount > 0
UNION ALL
SELECT 'cbd_congestion_fee',
    ROUND(SUM(cbd_congestion_fee), 2),
    ROUND(100.0 * SUM(cbd_congestion_fee)   / SUM(total_amount), 2)
FROM trips WHERE total_amount > 0
ORDER BY total DESC;


-- ── Q12 ── Zone-pair route popularity & yield ─────────────────
-- Answers: Which origin → destination pairs are most common and lucrative?
-- Technique: Composite GROUP BY, HAVING, derived trip duration

SELECT
    PULocationID,
    DOLocationID,
    PULocationID || ' → ' || DOLocationID                   AS route,
    COUNT(*)                                                 AS trips,
    ROUND(AVG(trip_distance), 2)                             AS avg_distance_mi,
    ROUND(AVG(fare_amount),   2)                             AS avg_fare,
    ROUND(AVG(tip_amount),    2)                             AS avg_tip,
    ROUND(AVG(total_amount),  2)                             AS avg_total,
    ROUND(AVG(
        EPOCH(tpep_dropoff_datetime - tpep_pickup_datetime) / 60.0
    ), 1)                                                    AS avg_duration_min
FROM trips
WHERE tpep_dropoff_datetime > tpep_pickup_datetime
GROUP BY PULocationID, DOLocationID
HAVING COUNT(*) >= 10
ORDER BY trips DESC
LIMIT 15;
