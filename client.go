// Package dadoslivres e a API publica da biblioteca dados-livres: harvest do
// indice offline de contratos assinados da API de Consulta v1 do PNCP, com
// saida configuravel (Postgres por padrao, ou planilha XLSX).
//
// Uso minimo (Postgres, configuracao 100% por codigo):
//
//	cliente, err := dadoslivres.New(ctx, dadoslivres.Options{
//		Postgres: &dadoslivres.PostgresConfig{
//			Host: "localhost", Port: "5432", User: "app", Password: "s3cret", Database: "dados_livres",
//		},
//	})
//	resumo, err := cliente.Harvest(ctx, types.Scope{States: []string{"GO"}})
package dadoslivres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/danyele/dados-livres/internal/client"
	"github.com/danyele/dados-livres/internal/database"
	"github.com/danyele/dados-livres/internal/harvest"
	"github.com/danyele/dados-livres/internal/postgres"
	"github.com/danyele/dados-livres/internal/progress"
	"github.com/danyele/dados-livres/internal/xlsx"
	"github.com/danyele/dados-livres/pncp/types"
	"github.com/danyele/dados-livres/storage"
)

// PostgresConfig descreve a conexao Postgres do backend "postgres".
type PostgresConfig struct {
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

// Options configura a construcao do Client de forma inteiramente por codigo:
// campos zerados caem em defaults hardcoded (ver cada campo). A configuracao e
// explicita — a biblioteca nao le mais variaveis de ambiente (isso fica a
// cargo dos comandos CLI, se preciso).
type Options struct {
	// Storage define o backend de saida. Valores: "postgres" (default) ou
	// "xlsx". Sobrepondo quando CustomStorage e preenchido.
	Storage string

	// CustomStorage permite injetar um backend proprio do consumidor.
	CustomStorage storage.Storage

	// XLSXPath e o arquivo de saida quando o backend e xlsx
	// (default: internal/database/xlsx/dados-livres.xlsx).
	XLSXPath string

	// Postgres define a conexao do backend "postgres". Obrigatorio quando
	// Storage=postgres e CustomStorage e nil — nao ha mais default fraco de
	// credencial.
	Postgres *PostgresConfig

	// BaseURL sobrepoe a URL interna da API de Consulta v1 do PNCP.
	// Default: https://pncp.gov.br/api/consulta/v1.
	BaseURL string

	// MaxConcurrency limita o numero de requisicoes simultaneas (default 1).
	MaxConcurrency int

	// DelayMS e o intervalo minimo entre requisicoes (default 0).
	DelayMS int

	// MaxPages limita o numero de paginas por escopo (0 = sem limite).
	MaxPages int

	// ApplyMigrations aplica as migrations embutidas ao abrir (default true,
	// apenas para Postgres). Use false e rode cmd/migrate para controle manual.
	ApplyMigrations *bool
}

// Client e o handle principal da biblioteca.
type Client struct {
	harvester *harvest.Harvester
	store     storage.Storage
	pool      *pgxpool.Pool
}

// New monta um Client conforme as Options (resolvendo defaults por codigo).
func New(ctx context.Context, options Options) (*Client, error) {
	storageType := options.Storage
	if storageType == "" {
		storageType = types.StoragePostgres
	}

	cliente := &Client{}

	var store storage.Storage
	if options.CustomStorage != nil {
		store = options.CustomStorage
	} else {
		switch storageType {
		case types.StorageXLSX:
			x, err := newXLSX(options)
			if err != nil {
				return nil, err
			}
			store = x
		case types.StoragePostgres:
			p, err := newPostgres(ctx, options)
			if err != nil {
				return nil, err
			}
			cliente.pool = p.pool
			store = p.store
		default:
			return nil, fmt.Errorf("persistencia desconhecida %q (use %q ou %q)",
				storageType, types.StoragePostgres, types.StorageXLSX)
		}
	}
	cliente.store = store

	httpClient := client.New(
		options.BaseURL,
		options.MaxConcurrency,
		options.DelayMS,
	)
	cliente.harvester = harvest.New(httpClient, store, progress.NewHarvestProgress(), harvest.Config{MaxPages: options.MaxPages})
	return cliente, nil
}

type postgresInternal struct {
	pool  *pgxpool.Pool
	store storage.Storage
}

func newPostgres(ctx context.Context, options Options) (*postgresInternal, error) {
	if options.Postgres == nil {
		return nil, fmt.Errorf("configuracao Postgres (Options.Postgres) e obrigatoria para o backend %q (ou injete Options.CustomStorage)", types.StoragePostgres)
	}
	p := options.Postgres
	cfg := database.Config{
		Host:            p.Host,
		Port:            p.Port,
		User:            p.User,
		Password:        p.Password,
		Database:        p.Database,
		MaxConns:        p.MaxConns,
		MinConns:        p.MinConns,
		MaxConnLifetime: p.MaxConnLifetime,
		MaxConnIdleTime: p.MaxConnIdleTime,
	}
	if cfg.Host == "" {
		cfg.Host = "127.0.0.1"
	}
	if cfg.Port == "" {
		cfg.Port = "5432"
	}
	if cfg.MaxConns <= 0 {
		cfg.MaxConns = 10
	}
	if cfg.MinConns <= 0 {
		cfg.MinConns = 2
	}
	if cfg.MaxConnLifetime == 0 {
		cfg.MaxConnLifetime = 30 * time.Minute
	}
	if cfg.MaxConnIdleTime == 0 {
		cfg.MaxConnIdleTime = 5 * time.Minute
	}
	pool, err := database.NewPool(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("conectar postgres: %w", err)
	}

	apply := true
	if options.ApplyMigrations != nil {
		apply = *options.ApplyMigrations
	}
	if apply {
		if err := database.Migrate(ctx, pool); err != nil && err != database.ErrNoPendingMigrations {
			pool.Close()
			return nil, fmt.Errorf("aplicar migracoes: %w", err)
		}
	}

	return &postgresInternal{pool: pool, store: postgres.New(pool)}, nil
}

func newXLSX(options Options) (storage.Storage, error) {
	path := options.XLSXPath
	if path == "" {
		path = types.XLSXDefaultPath
	}
	return xlsx.New(path)
}

// Close libera os recursos (pool Postgres, arquivo aberto).
func (c *Client) Close() {
	if c.pool != nil {
		c.pool.Close()
	}
}

// -----------------------------------------------------------------------------
// Operacoes
// -----------------------------------------------------------------------------

// Harvest pagina o escopo informado e persiste os contratos publicados.
func (c *Client) Harvest(ctx context.Context, scope types.Scope) (*types.HarvestSummary, error) {
	return c.harvester.Harvest(ctx, scope)
}

// Progress devolve o andamento atual do ultimo/atual harvest.
func (c *Client) Progress() types.ProgressEvent {
	return c.harvester.ProgressEvent()
}

// Status devolve o estado agregado do indice (registros, janela, coberturas).
func (c *Client) Status(ctx context.Context) (*types.IndexStatus, error) {
	return c.harvester.Status(ctx)
}

// ListCoverages devolve as coberturas registradas (escopos harvestados).
func (c *Client) ListCoverages(ctx context.Context) ([]types.Coverage, error) {
	return c.harvester.ListCoverages(ctx)
}

// CountRecords conta os contratos persistidos.
func (c *Client) CountRecords(ctx context.Context) (int, error) {
	return c.harvester.CountRecords(ctx)
}

// SuppliersByName busca fornecedores por razao social (requer Postgres).
func (c *Client) SuppliersByName(ctx context.Context, name string, limit int) ([]types.Supplier, error) {
	return c.harvester.SuppliersByName(ctx, name, limit)
}

// ContractsBySupplier lista os contratos de um fornecedor (requer Postgres).
func (c *Client) ContractsBySupplier(ctx context.Context, supplierID, groupBy string, limit int) (*types.SupplierContractsResult, error) {
	return c.harvester.ContractsBySupplier(ctx, supplierID, groupBy, limit)
}

// SearchContracts retorna os contratos publicados no escopo (requer Postgres).
func (c *Client) SearchContracts(ctx context.Context, filter types.SearchFilter) ([]types.ContractSearch, error) {
	return c.harvester.SearchContracts(ctx, filter)
}

// SearchProcurements retorna as contratacoes publicadas no escopo (requer
// Postgres).
func (c *Client) SearchProcurements(ctx context.Context, filter types.SearchFilter) ([]types.ProcurementSearch, error) {
	return c.harvester.SearchProcurements(ctx, filter)
}
