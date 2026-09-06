package database

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// ErrNoPendingMigrations indica que nao ha o que aplicar.
var ErrNoPendingMigrations = errors.New("nenhuma migracao pendente")

// Migrate aplica as migrations embutidas (arquivos NNN_descricao.up.sql) que
// ainda nao constam em dados_livres_migrations. Cada migracao roda em uma
// transacao propria.
func Migrate(ctx context.Context, db DB) error {
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return err
	}

	pending, err := pendingMigrations(ctx, db)
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return ErrNoPendingMigrations
	}

	for _, version := range pending {
		if err := apply(ctx, db, version); err != nil {
			return fmt.Errorf("migracao %s: %w", version, err)
		}
	}
	return nil
}

func ensureMigrationsTable(ctx context.Context, db DB) error {
	_, err := db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS dados_livres_migrations (
			versao TEXT PRIMARY KEY,
			aplicada_em TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`)
	return err
}

func pendingMigrations(ctx context.Context, db DB) ([]string, error) {
	applied := map[string]bool{}
	rows, err := db.Query(ctx, `SELECT versao FROM dados_livres_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		applied[v] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	var out []string
	for _, name := range upMigrations() {
		if !applied[name] {
			out = append(out, name)
		}
	}
	return out, nil
}

func apply(ctx context.Context, db DB, version string) error {
	sql, err := readUp(version)
	if err != nil {
		return err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO dados_livres_migrations (versao) VALUES ($1)`, version); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// upMigrations lista os arquivos de migracao (NNN_descricao.up.sql) em ordem
// numerica.
func upMigrations() []string {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil
	}
	var up []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			up = append(up, e.Name())
		}
	}
	sort.Strings(up)
	return up
}

func readUp(version string) (string, error) {
	b, err := migrationsFS.ReadFile("migrations/" + version)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
