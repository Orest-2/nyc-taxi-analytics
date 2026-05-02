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