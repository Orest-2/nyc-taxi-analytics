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