-- Add new table to keep track of processed ATProto events using a sequence number
create table if not exists sequence (
    id integer primary key check (id = 0),
    seq integer not null
);

insert or ignore into sequence (id, seq) values (0, -1);
