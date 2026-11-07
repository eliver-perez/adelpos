package database

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"local/ad-el-pos/server/internal/config"
)

func Open(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	c, err := pgxpool.ParseConfig(cfg.ConnectionString())
	if err != nil {
		return nil, errors.New("configuración PostgreSQL inválida")
	}
	c.MaxConns = cfg.MaxConnections
	c.MinConns = 0
	c.MaxConnLifetime = 30 * time.Minute
	c.MaxConnIdleTime = 5 * time.Minute
	c.HealthCheckPeriod = 30 * time.Second
	c.ConnConfig.RuntimeParams["timezone"] = "UTC"
	return pgxpool.NewWithConfig(ctx, c)
}

type Health struct {
	Postgres string `json:"postgres"`
	Schema   string `json:"schema"`
}

func Probe(ctx context.Context, pool *pgxpool.Pool) Health {
	h := Health{Postgres: "unavailable", Schema: "unknown"}
	if err := pool.Ping(ctx); err != nil {
		return h
	}
	h.Postgres = "available"
	if err := CheckSchema(ctx, pool); err != nil {
		h.Schema = "incompatible"
	} else {
		h.Schema = "compatible"
	}
	return h
}
