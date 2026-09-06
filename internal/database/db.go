// Package database oferece a abstração mínima de acesso ao Postgres usada pela
// biblioteca: conexao via env (DB_* padrão, com DADOS_LIVRES_DB_* como override)
// e o runner de migracoes embutidas das tabelas dados_livres_*.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danyele/dados-livres/internal/env"
)

// DB é a superficie de banco consumida pelos repositorios e migracoes.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

// Config descreve a conexao Postgres.
type Config struct {
	Host            string
	Port            string
	User            string
	Password        string
	Database        string
	MaxConns        int32
	MinConns        int32
	MaxConnLifetime time.Duration
	MaxConnIdleTime time.Duration
}

// ConfigFromEnv monta a Config a partir das variaveis padrao DB_* (padrão
// do oicp), permitindo override com DADOS_LIVRES_DB_*. Sem default de senha:
// a conexao falha de forma explicita se DB_PASSWORD nao for informado.
func ConfigFromEnv() Config {
	return Config{
		Host:            env.String("DADOS_LIVRES_DB_HOST", env.String("DB_HOST", "localhost")),
		Port:            env.String("DADOS_LIVRES_DB_PORT", env.String("DB_PORT", "5432")),
		User:            env.String("DADOS_LIVRES_DB_USER", env.String("DB_USER", "postgres")),
		Password:        env.String("DADOS_LIVRES_DB_PASSWORD", env.String("DB_PASSWORD", "")),
		Database:        env.String("DADOS_LIVRES_DB_NAME", env.String("DB_NAME", "tse_data")),
		MaxConns:        env.Int32("DADOS_LIVRES_DB_MAX_CONNS", 10),
		MinConns:        2,
		MaxConnLifetime: 30 * time.Minute,
		MaxConnIdleTime: 5 * time.Minute,
	}
}

// NewPool cria um pgxpool conectado (valida com Ping).
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable&pool_max_conns=%d&pool_min_conns=%d",
		cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database,
		cfg.MaxConns, cfg.MinConns,
	)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("new pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return pool, nil
}
