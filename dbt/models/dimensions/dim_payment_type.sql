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