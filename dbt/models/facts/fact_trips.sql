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
        'pickup_zone_id',
        'trip_distance',
        'total_amount'
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
    greatest(t.total_amount - t.fare_amount, 0)  as total_surcharges,
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