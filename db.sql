CREATE TABLE data
(
    block_number     BIGINT PRIMARY KEY,
    data             BYTEA,
    submitted_height BIGINT,
    commitment       BYTEA,
    proof            BYTEA,
    submit_to_eth    BOOLEAN DEFAULT false
);
