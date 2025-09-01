create table orders (
                        id            uuid primary key,
                        client_id     text not null,
                        symbol        text not null,
                        side          text not null check (side in ('BUY','SELL')),
                        type          text not null check (type in ('MARKET','LIMIT')),
                        price         numeric(38, 8),              -- null для MARKET, либо >0
                        quantity      numeric(38, 8) not null check (quantity > 0),
                        remaining     numeric(38, 8) not null check (remaining >= 0),
                        status        text not null check (status in ('OPEN','PARTIALLY_FILLED','FILLED','CANCELLED')),
                        created_at    timestamptz not null default now(),
                        updated_at    timestamptz not null default now()
);

create index on orders (symbol, side, status, price, created_at);
create index on orders (client_id, id);

create table trades (
                        id          uuid primary key,
                        symbol      text not null,
                        buy_order   uuid not null references orders(id),
                        sell_order  uuid not null references orders(id),
                        price       numeric(38, 8) not null,
                        quantity    numeric(38, 8) not null,
                        executed_at timestamptz not null default now()
);

create or replace function set_updated_at() returns trigger as $$
begin
  new.updated_at = now();
return new;
end $$ language plpgsql;

create trigger orders_set_updated_at
    before update on orders
    for each row execute function set_updated_at();
