CREATE TABLE data
(
    batch_number      BIGINT PRIMARY KEY,
    data              BYTEA,
    committed_height  BIGINT,
    committed_tx_hash BYTEA,
    commitment        BYTEA,
    submit_to_eth     BOOLEAN DEFAULT false
);
