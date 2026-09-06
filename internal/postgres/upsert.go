// Pacote postgres implementa storage.Storage e storage.Searcher sobre Postgres,
// nas tabelas dados_livres_*.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/danyele/dados-livres/internal/database"
	"github.com/danyele/dados-livres/pncp/types"
)

// UpsertBatchSize define o numero de linhas por INSERT com UNNEST.
const UpsertBatchSize = 500

// Store implementa as interfaces de persistencia sobre Postgres.
type Store struct {
	db database.DB
}

// New cria um Store Postgres sobre a base informada.
func New(db database.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Ping(ctx context.Context) error {
	return s.db.Ping(ctx)
}

// -----------------------------------------------------------------------------
// Upserts
// -----------------------------------------------------------------------------

// UpsertContracts grava/atualiza contratos em lote.
func (s *Store) UpsertContracts(ctx context.Context, contracts []types.Contract) error {
	for _, batch := range chunk(contracts, UpsertBatchSize) {
		if err := s.upsertContractBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

const sqlUpsertContract = `
INSERT INTO dados_livres_pncp_contrato (
    numero_controle_pncp, numero_controle_pncp_compra, ano_contrato,
    sequencial_contrato, objeto_contrato, ni_fornecedor, tipo_pessoa,
    nome_razao_social_fornecedor, ni_fornecedor_sub_contratado,
    nome_fornecedor_sub_contratado, valor_inicial, valor_global,
    valor_acumulado, data_assinatura, data_vigencia_inicio, data_vigencia_fim,
    data_publicacao_pncp, data_atualizacao_global, orgao_cnpj,
    orgao_razao_social, codigo_ibge, municipio_nome, uf_sigla, tipo_contrato,
    dados_json
)
SELECT * FROM UNNEST(
    $1::text[], $2::text[], $3::int[],
    $4::int[], $5::text[], $6::text[], $7::text[],
    $8::text[], $9::text[],
    $10::text[], $11::numeric[], $12::numeric[],
    $13::numeric[], $14::timestamptz[], $15::timestamptz[], $16::timestamptz[],
    $17::timestamptz[], $18::timestamptz[], $19::text[],
    $20::text[], $21::text[], $22::text[], $23::text[], $24::text[],
    $25::jsonb[]
)
ON CONFLICT (numero_controle_pncp) DO UPDATE SET
    numero_controle_pncp_compra = EXCLUDED.numero_controle_pncp_compra,
    ano_contrato = EXCLUDED.ano_contrato,
    sequencial_contrato = EXCLUDED.sequencial_contrato,
    objeto_contrato = EXCLUDED.objeto_contrato,
    ni_fornecedor = EXCLUDED.ni_fornecedor,
    tipo_pessoa = EXCLUDED.tipo_pessoa,
    nome_razao_social_fornecedor = EXCLUDED.nome_razao_social_fornecedor,
    ni_fornecedor_sub_contratado = EXCLUDED.ni_fornecedor_sub_contratado,
    nome_fornecedor_sub_contratado = EXCLUDED.nome_fornecedor_sub_contratado,
    valor_inicial = EXCLUDED.valor_inicial,
    valor_global = EXCLUDED.valor_global,
    valor_acumulado = EXCLUDED.valor_acumulado,
    data_assinatura = EXCLUDED.data_assinatura,
    data_vigencia_inicio = EXCLUDED.data_vigencia_inicio,
    data_vigencia_fim = EXCLUDED.data_vigencia_fim,
    data_publicacao_pncp = EXCLUDED.data_publicacao_pncp,
    data_atualizacao_global = EXCLUDED.data_atualizacao_global,
    orgao_cnpj = EXCLUDED.orgao_cnpj,
    orgao_razao_social = EXCLUDED.orgao_razao_social,
    codigo_ibge = EXCLUDED.codigo_ibge,
    municipio_nome = EXCLUDED.municipio_nome,
    uf_sigla = EXCLUDED.uf_sigla,
    tipo_contrato = EXCLUDED.tipo_contrato,
    dados_json = EXCLUDED.dados_json,
    updated_at = NOW()`

func (s *Store) upsertContractBatch(ctx context.Context, batch []types.Contract) error {
	n := len(batch)
	columns := make([][]any, 25)
	for i := range columns {
		columns[i] = make([]any, n)
	}
	for i, row := range batch {
		columns[0][i] = row.ControlNumberPNCP
		columns[1][i] = row.PurchaseControlNumber
		columns[2][i] = row.ContractYear
		columns[3][i] = row.ContractSequential
		columns[4][i] = row.ContractObject
		columns[5][i] = row.SupplierID
		columns[6][i] = row.PersonType
		columns[7][i] = row.SupplierName
		columns[8][i] = row.SubcontractorID
		columns[9][i] = row.SubcontractorName
		columns[10][i] = row.InitialValue
		columns[11][i] = row.GlobalValue
		columns[12][i] = row.AccumulatedValue
		columns[13][i] = row.SignatureDate
		columns[14][i] = row.StartDate
		columns[15][i] = row.EndDate
		columns[16][i] = row.PublicationDate
		columns[17][i] = row.UpdateDate
		columns[18][i] = row.AgencyCNPJ
		columns[19][i] = row.AgencyName
		columns[20][i] = row.IBGECode
		columns[21][i] = row.MunicipalityName
		columns[22][i] = row.StateAcronym
		columns[23][i] = row.ContractType
		columns[24][i] = rawJSONString(row.RawJSON)
	}
	args := make([]any, 25)
	for i := range columns {
		args[i] = columns[i]
	}
	_, err := s.db.Exec(ctx, sqlUpsertContract, args...)
	return err
}

// UpsertProcurements grava/atualiza contratacoes em lote.
func (s *Store) UpsertProcurements(ctx context.Context, procurements []types.Procurement) error {
	for _, batch := range chunk(procurements, UpsertBatchSize) {
		if err := s.upsertProcurementBatch(ctx, batch); err != nil {
			return err
		}
	}
	return nil
}

const sqlUpsertProcurement = `
INSERT INTO dados_livres_pncp_contratacao (
    numero_controle_pncp, numero_compra, ano_compra, sequencial_compra,
    modalidade_id, modalidade_nome, modo_disputa_id, modo_disputa_nome,
    situacao_compra_id, situacao_compra_nome, objeto_compra,
    valor_total_estimado, valor_total_homologado,
    data_publicacao_pncp, data_inclusao, data_atualizacao,
    data_atualizacao_global, data_abertura_proposta, data_encerramento_proposta,
    orgao_cnpj, orgao_razao_social, codigo_ibge, municipio_nome, uf_sigla,
    unidade_nome, tipo_instrumento_convocatorio_codigo,
    tipo_instrumento_convocatorio_nome, srp, emenda_parlamentar,
    processo, link_processo_eletronico, link_sistema_origem, usuario_nome,
    dados_json
)
SELECT * FROM UNNEST(
    $1::text[], $2::text[], $3::int[], $4::int[],
    $5::int[], $6::text[], $7::int[], $8::text[],
    $9::int[], $10::text[], $11::text[],
    $12::numeric[], $13::numeric[],
    $14::timestamptz[], $15::timestamptz[], $16::timestamptz[],
    $17::timestamptz[], $18::timestamptz[], $19::timestamptz[],
    $20::text[], $21::text[], $22::text[], $23::text[], $24::text[],
    $25::text[], $26::int[],
    $27::text[], $28::boolean[], $29::boolean[],
    $30::text[], $31::text[], $32::text[], $33::text[],
    $34::jsonb[]
)
ON CONFLICT (numero_controle_pncp) DO UPDATE SET
    numero_compra = EXCLUDED.numero_compra,
    ano_compra = EXCLUDED.ano_compra,
    sequencial_compra = EXCLUDED.sequencial_compra,
    modalidade_id = EXCLUDED.modalidade_id,
    modalidade_nome = EXCLUDED.modalidade_nome,
    modo_disputa_id = EXCLUDED.modo_disputa_id,
    modo_disputa_nome = EXCLUDED.modo_disputa_nome,
    situacao_compra_id = EXCLUDED.situacao_compra_id,
    situacao_compra_nome = EXCLUDED.situacao_compra_nome,
    objeto_compra = EXCLUDED.objeto_compra,
    valor_total_estimado = EXCLUDED.valor_total_estimado,
    valor_total_homologado = EXCLUDED.valor_total_homologado,
    data_publicacao_pncp = EXCLUDED.data_publicacao_pncp,
    data_inclusao = EXCLUDED.data_inclusao,
    data_atualizacao = EXCLUDED.data_atualizacao,
    data_atualizacao_global = EXCLUDED.data_atualizacao_global,
    data_abertura_proposta = EXCLUDED.data_abertura_proposta,
    data_encerramento_proposta = EXCLUDED.data_encerramento_proposta,
    orgao_cnpj = EXCLUDED.orgao_cnpj,
    orgao_razao_social = EXCLUDED.orgao_razao_social,
    codigo_ibge = EXCLUDED.codigo_ibge,
    municipio_nome = EXCLUDED.municipio_nome,
    uf_sigla = EXCLUDED.uf_sigla,
    unidade_nome = EXCLUDED.unidade_nome,
    tipo_instrumento_convocatorio_codigo = EXCLUDED.tipo_instrumento_convocatorio_codigo,
    tipo_instrumento_convocatorio_nome = EXCLUDED.tipo_instrumento_convocatorio_nome,
    srp = EXCLUDED.srp,
    emenda_parlamentar = EXCLUDED.emenda_parlamentar,
    processo = EXCLUDED.processo,
    link_processo_eletronico = EXCLUDED.link_processo_eletronico,
    link_sistema_origem = EXCLUDED.link_sistema_origem,
    usuario_nome = EXCLUDED.usuario_nome,
    dados_json = EXCLUDED.dados_json,
    updated_at = NOW()`

func (s *Store) upsertProcurementBatch(ctx context.Context, batch []types.Procurement) error {
	n := len(batch)
	columns := make([][]any, 34)
	for i := range columns {
		columns[i] = make([]any, n)
	}
	for i, row := range batch {
		columns[0][i] = row.ControlNumberPNCP
		columns[1][i] = row.PurchaseNumber
		columns[2][i] = row.PurchaseYear
		columns[3][i] = row.PurchaseSequential
		columns[4][i] = row.ModalityID
		columns[5][i] = row.ModalityName
		columns[6][i] = row.DisputeModeID
		columns[7][i] = row.DisputeModeName
		columns[8][i] = row.PurchaseStatusID
		columns[9][i] = row.PurchaseStatusName
		columns[10][i] = row.PurchaseObject
		columns[11][i] = row.EstimatedTotalValue
		columns[12][i] = row.ApprovedTotalValue
		columns[13][i] = row.PublicationDate
		columns[14][i] = row.InclusionDate
		columns[15][i] = row.UpdateDate
		columns[16][i] = row.GlobalUpdateDate
		columns[17][i] = row.BidOpeningDate
		columns[18][i] = row.BidClosingDate
		columns[19][i] = row.AgencyCNPJ
		columns[20][i] = row.AgencyName
		columns[21][i] = row.IBGECode
		columns[22][i] = row.MunicipalityName
		columns[23][i] = row.StateAcronym
		columns[24][i] = row.UnitName
		columns[25][i] = row.InstrumentTypeCode
		columns[26][i] = row.InstrumentTypeName
		columns[27][i] = row.SRP
		columns[28][i] = row.BudgetAmendment
		columns[29][i] = row.Process
		columns[30][i] = row.ElectronicProcessLink
		columns[31][i] = row.SourceSystemLink
		columns[32][i] = row.UserName
		columns[33][i] = rawJSONString(row.RawJSON)
	}
	args := make([]any, 34)
	for i := range columns {
		args[i] = columns[i]
	}
	_, err := s.db.Exec(ctx, sqlUpsertProcurement, args...)
	return err
}

func chunk[T any](rows []T, size int) [][]T {
	if len(rows) == 0 {
		return nil
	}
	var out [][]T
	for len(rows) > size {
		out = append(out, rows[:size])
		rows = rows[size:]
	}
	return append(out, rows)
}

// rawJSONString converte o registro cru em texto JSON (nil -> NULL) para o
// array jsonb do UNNEST.
func rawJSONString(data json.RawMessage) any {
	if len(data) == 0 {
		return nil
	}
	return string(data)
}

// -----------------------------------------------------------------------------
// Metadados e status
// -----------------------------------------------------------------------------

// RecordCoverage grava/atualiza a linha de cobertura de um escopo. Quando o
// totalRegistros informado e nil (ex.: harvest retomado, sem pagina 1),
// preserva o total ja conhecido da cobertura anterior.
func (s *Store) RecordCoverage(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int, status string, totalRecords *int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO dados_livres_pncp_meta (entidade, uf, municipio, orgao_cnpj, modalidade, data_inicio, data_fim, status, total_registros, atualizado_em)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (entidade, uf, municipio, modalidade, orgao_cnpj, data_inicio, data_fim)
		DO UPDATE SET
			status = EXCLUDED.status,
			total_registros = COALESCE(dados_livres_pncp_meta.total_registros, EXCLUDED.total_registros),
			pagina_erro = NULL,
			atualizado_em = EXCLUDED.atualizado_em,
			updated_at = NOW()`,
		entity, state, municipality, agencyCNPJ, modality, startDate, endDate, status, totalRecords)
	return err
}

// RecordPageProgress registra o andamento do harvest apos cada pagina.
func (s *Store) RecordPageProgress(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, page int, totalRecords *int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO dados_livres_pncp_meta (entidade, uf, municipio, orgao_cnpj, modalidade, data_inicio, data_fim, status, ultima_pagina_ok, total_registros, atualizado_em)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, NOW())
		ON CONFLICT (entidade, uf, municipio, modalidade, orgao_cnpj, data_inicio, data_fim)
		DO UPDATE SET
			status = 'parcial',
			ultima_pagina_ok = GREATEST(COALESCE(dados_livres_pncp_meta.ultima_pagina_ok, 0), EXCLUDED.ultima_pagina_ok),
			total_registros = COALESCE(dados_livres_pncp_meta.total_registros, EXCLUDED.total_registros),
			atualizado_em = NOW(),
			updated_at = NOW()`,
		entity, state, municipality, agencyCNPJ, modality, startDate, endDate, types.StatusPartial, page, totalRecords)
	return err
}

// RecordPageFailure marca o escopo com status 'error' e a pagina da falha.
func (s *Store) RecordPageFailure(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, errorPage int) error {
	_, err := s.db.Exec(ctx, `
		INSERT INTO dados_livres_pncp_meta (entidade, uf, municipio, orgao_cnpj, modalidade, data_inicio, data_fim, status, pagina_erro, atualizado_em)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (entidade, uf, municipio, modalidade, orgao_cnpj, data_inicio, data_fim)
		DO UPDATE SET
			status = 'error',
			pagina_erro = EXCLUDED.pagina_erro,
			atualizado_em = NOW(),
			updated_at = NOW()`,
		entity, state, municipality, agencyCNPJ, modality, startDate, endDate, types.StatusError, errorPage)
	return err
}

// LastPageOK devolve a ultima pagina persistida do escopo (0 = nunca).
func (s *Store) LastPageOK(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (int, error) {
	var last int
	err := s.db.QueryRow(ctx, `
		SELECT ultima_pagina_ok FROM dados_livres_pncp_meta
		WHERE entidade = $1 AND uf = $2 AND municipio = $3
		  AND orgao_cnpj = $4 AND modalidade = $5 AND data_inicio = $6 AND data_fim = $7`,
		entity, state, municipality, agencyCNPJ, modality, startDate, endDate).Scan(&last)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return last, nil
}

// CoverageCompleted informa se o escopo ja foi concluido (dedup).
func (s *Store) CoverageCompleted(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (bool, error) {
	var exists bool
	err := s.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM dados_livres_pncp_meta
			WHERE entidade = $1 AND uf = $2 AND municipio = $3
			  AND orgao_cnpj = $4 AND modalidade = $5 AND data_inicio = $6 AND data_fim = $7 AND status = $8
		)`, entity, state, municipality, agencyCNPJ, modality, startDate, endDate, types.StatusCompleted).Scan(&exists)
	return exists, err
}

// ListCoverages le as linhas estruturadas da pncp_meta.
func (s *Store) ListCoverages(ctx context.Context) ([]types.Coverage, error) {
	rows, err := s.db.Query(ctx, `
		SELECT entidade, uf, municipio, orgao_cnpj, modalidade, data_inicio, data_fim, status, total_registros, ultima_pagina_ok, pagina_erro, atualizado_em
		FROM dados_livres_pncp_meta
		ORDER BY entidade, data_inicio, data_fim`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.Coverage{}
	for rows.Next() {
		var coverage types.Coverage
		if err := rows.Scan(
			&coverage.Entity,
			&coverage.State,
			&coverage.Municipality,
			&coverage.AgencyCNPJ,
			&coverage.Modality,
			&coverage.StartDate,
			&coverage.EndDate,
			&coverage.Status,
			&coverage.TotalRecords,
			&coverage.LastPageOK,
			&coverage.ErrorPage,
			&coverage.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, coverage)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// CountRecords conta as linhas de contratos persistidos.
func (s *Store) CountRecords(ctx context.Context) (int, error) {
	var total int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM dados_livres_pncp_contrato`).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) countProcurements(ctx context.Context) (int, error) {
	var total int
	err := s.db.QueryRow(ctx, `SELECT COUNT(*) FROM dados_livres_pncp_contratacao`).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total, nil
}

func (s *Store) tableExists(ctx context.Context, name string) (bool, error) {
	var total int
	err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public' AND table_name = $1`, name).Scan(&total)
	if err != nil {
		return false, err
	}
	return total > 0, nil
}

// Status monta o estado agregado do indice.
func (s *Store) Status(ctx context.Context) (*types.IndexStatus, error) {
	hasContract, err := s.tableExists(ctx, "dados_livres_pncp_contrato")
	if err != nil {
		return nil, err
	}
	hasProcurement, err := s.tableExists(ctx, "dados_livres_pncp_contratacao")
	if err != nil {
		return nil, err
	}
	if !hasContract && !hasProcurement {
		return &types.IndexStatus{Exists: false}, nil
	}

	records := 0
	if hasContract {
		n, err := s.CountRecords(ctx)
		if err != nil {
			return nil, err
		}
		records += n
	}
	if hasProcurement {
		n, err := s.countProcurements(ctx)
		if err != nil {
			return nil, err
		}
		records += n
	}

	status := &types.IndexStatus{
		Exists:    true,
		Records:   records,
		Coverages: []types.Coverage{},
	}

	var startDate, endDate *string
	var updatedAt *time.Time
	err = s.db.QueryRow(ctx, `
		SELECT MIN(data_inicio), MAX(data_fim), MAX(atualizado_em)
		FROM dados_livres_pncp_meta
		WHERE status = $1`, types.StatusCompleted).Scan(&startDate, &endDate, &updatedAt)
	if err != nil {
		return nil, err
	}
	if startDate != nil {
		status.Window.StartDate = *startDate
	}
	if endDate != nil {
		status.Window.EndDate = *endDate
	}
	status.UpdatedAt = updatedAt

	coverages, err := s.ListCoverages(ctx)
	if err != nil {
		return nil, err
	}
	status.Coverages = coverages
	return status, nil
}
