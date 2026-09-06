// Package storage define as interfaces de saida da biblioteca dados-livres.
// Implementacoes fornecidas: Postgres (default) e planilha XLSX. Outros
// backends podem ser implementados pelo consumidor e injetados via
// dadoslivres.Options.
package storage

import (
	"context"

	"github.com/danyele/dados-livres/pncp/types"
)

// ErrSearchNotSupported e retornado por backends que nao implementam as buscas
// indexadas (ex.: planilha XLSX, que so persiste os dados).
var ErrSearchNotSupported = &UnsupportedError{}

// UnsupportedError indica que uma operacao nao e suportada pelo backend.
type UnsupportedError struct{}

func (e *UnsupportedError) Error() string {
	return "operacao nao suportada por este backend de persistencia"
}

// Storage e o conjunto minimo de operacoes que o harvest exige para gravar
// contratos e contratacoes, rastrear progresso por pagina e expor estado. O
// escopo de uma cobertura e identificado por (entidade, uf, municipio,
// orgao_cnpj, modalidade, data_inicio, data_fim): agencyCNPJ distingue as
// varreduras de contratos de cada orgao.
type Storage interface {
	// UpsertContracts grava/atualiza contratos em lote (chave
	// numero_controle_pncp).
	UpsertContracts(ctx context.Context, contracts []types.Contract) error

	// UpsertProcurements grava/atualiza contratacoes em lote (chave
	// numero_controle_pncp).
	UpsertProcurements(ctx context.Context, procurements []types.Procurement) error

	// RecordCoverage marca um escopo como concluido (ou outro status), com o
	// total de registros esperado informado pela API.
	RecordCoverage(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int, status string, totalRecords *int) error

	// RecordPageProgress registra a ultima pagina persistida com sucesso
	// (status 'parcial').
	RecordPageProgress(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, page int, totalRecords *int) error

	// RecordPageFailure marca o escopo com status 'error' e a pagina da falha.
	RecordPageFailure(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, errorPage int) error

	// LastPageOK devolve a ultima pagina persistida do escopo (0 = nunca).
	LastPageOK(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (int, error)

	// CoverageCompleted informa se o escopo ja foi concluido (dedup).
	CoverageCompleted(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (bool, error)

	// ListCoverages le todas as linhas de cobertura.
	ListCoverages(ctx context.Context) ([]types.Coverage, error)

	// CountRecords conta as linhas de contratos persistidos.
	CountRecords(ctx context.Context) (int, error)

	// Status monta o estado agregado do indice.
	Status(ctx context.Context) (*types.IndexStatus, error)
}

// Searcher e o conjunto de buscas indexadas, suportado apenas pelo backend
// Postgres. Backends que nao implementam retornam storage.ErrSearchNotSupported.
type Searcher interface {
	SuppliersByName(ctx context.Context, name string, limit int) ([]types.Supplier, error)
	ContractsBySupplier(ctx context.Context, supplierID, groupBy string, limit int) (*types.SupplierContractsResult, error)
	SearchContracts(ctx context.Context, filter types.SearchFilter) ([]types.ContractSearch, error)
	SearchProcurements(ctx context.Context, filter types.SearchFilter) ([]types.ProcurementSearch, error)
}
