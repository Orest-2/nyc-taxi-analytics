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