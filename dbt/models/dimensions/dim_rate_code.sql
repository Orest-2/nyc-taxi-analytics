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