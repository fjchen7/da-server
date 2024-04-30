CREATE TABLE data
(
    batch_number      BIGINT,
    data              BYTEA,
    committed_height  BIGINT,
    committed_tx_hash BYTEA,
    commitment        BYTEA,
    submit_to_eth     BOOLEAN DEFAULT false,
    created_at        TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ,
    PRIMARY KEY (batch_number)
);

CREATE TABLE txs
(
    batch_number   BIGINT,
    hash           BYTEA,
    nonce          BIGINT,
    confirm_status TEXT,
    sent_time      TIMESTAMPTZ,
    created_at     TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ,
    PRIMARY KEY (batch_number, hash)
);
