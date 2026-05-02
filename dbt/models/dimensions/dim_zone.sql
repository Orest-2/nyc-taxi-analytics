select
    "LocationID"                    as zone_id,
    "Borough"                       as borough,
    "Zone"                          as zone_name,
    "service_zone"                  as service_zone,

    -- convenience flags used in fact_trips downstream
    "service_zone" = 'Airports'     as is_airport,
    "Borough" = 'Manhattan'         as is_manhattan

from {{ ref('taxi_zone_lookup') }}