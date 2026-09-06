// Command migrate aplica as migrations embutidas das tabelas dados_livres_*
// no Postgres configurado pelas variaveis DB_* (ou DADOS_LIVRES_DB_*).
package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/danyele/dados-livres/internal/database"
)

func main() {
	ctx := context.Background()

	cfg := database.ConfigFromEnv()
	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao conectar: %v\n", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := database.Migrate(ctx, pool); err != nil {
		if errors.Is(err, database.ErrNoPendingMigrations) {
			fmt.Println("nenhuma migracao pendente")
			return
		}
		fmt.Fprintf(os.Stderr, "erro ao migrar: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("migracoes aplicadas")
}
