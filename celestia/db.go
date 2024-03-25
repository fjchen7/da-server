package celestia

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/lib/pq"
	"os"
	"strconv"
)

type DBConfig struct {
	host     string
	port     uint64
	user     string
	password string
	dbname   string
}

type DBConnector struct {
	db sql.DB
}

func NewConnector(cfg DBConfig) (*DBConnector, error) {
	psqlconn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable", cfg.host, cfg.port, cfg.user, cfg.password, cfg.dbname)
	db, err := sql.Open("postgres", psqlconn)
	if err != nil {
		return nil, err
	}
	return &DBConnector{db: *db}, nil
}

func NewConnectorFromEnv() (*DBConnector, error) {
	portStr := os.Getenv("POSTGRESQL_PORT")
	port, err := strconv.ParseUint(portStr, 10, 32)
	if err != nil {
		return nil, err
	}
	cfg := DBConfig{
		host:     os.Getenv("POSTGRESQL_HOST"),
		port:     port,
		user:     os.Getenv("POSTGRESQL_USER"),
		password: os.Getenv("POSTGRESQL_PASSWORD"),
		dbname:   os.Getenv("POSTGRESQL_DBNAME"),
	}
	return NewConnector(cfg)
}

func (c *DBConnector) Close() {
	c.db.Close()
}

func (c *DBConnector) StoreDAProof(proof DAProof) error {
	proofJson, err := json.Marshal(proof.Proof)
	if err != nil {
		return err
	}
	_, err = c.db.Exec("INSERT INTO DAProof (height, commitment, proof) VALUES ($1, $2, $3)", proof.SubmitHeight, proof.Commitment, proofJson)
	if err != nil {
		return err
	}
	return nil
}
