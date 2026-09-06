package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/danyele/dados-livres/internal/strutil"
	"github.com/danyele/dados-livres/pncp/types"
)

// -----------------------------------------------------------------------------
// Helpers de busca
// -----------------------------------------------------------------------------

// clampLimit limita o limite entre 1 e 200 (fallback 25).
func clampLimit(limit int) int {
	if limit <= 0 {
		return 25
	}
	if limit > 200 {
		return 200
	}
	return limit
}

// -----------------------------------------------------------------------------
// SuppliersByName
// -----------------------------------------------------------------------------

// SuppliersByName busca fornecedores por razao social com total de contratos
// e valor agregado.
func (s *Store) SuppliersByName(ctx context.Context, name string, limit int) ([]types.Supplier, error) {
	limit = clampLimit(limit)
	rows, err := s.db.Query(ctx, `
		SELECT ni_fornecedor, nome_razao_social_fornecedor AS nome,
			COUNT(*) AS contratos,
			SUM(COALESCE(valor_global, valor_inicial, 0)) AS valor_total
		FROM dados_livres_pncp_contrato
		WHERE nome_razao_social_fornecedor ILIKE $1
		GROUP BY ni_fornecedor, nome_razao_social_fornecedor
		ORDER BY valor_total DESC
		LIMIT $2`, "%"+name+"%", limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.Supplier{}
	for rows.Next() {
		var item types.Supplier
		if err := rows.Scan(&item.SupplierID, &item.Name, &item.Contracts, &item.TotalValue); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// -----------------------------------------------------------------------------
// ContractsBySupplier
// -----------------------------------------------------------------------------

// ContractsBySupplier lista os contratos de um fornecedor (niFornecedor) e
// agrega o gasto por orgao (default), municipio ou uf.
func (s *Store) ContractsBySupplier(ctx context.Context, supplierID, groupBy string, limit int) (*types.SupplierContractsResult, error) {
	limit = clampLimit(limit)
	supplierID = strutil.OnlyDigits(supplierID)
	if groupBy == "" {
		groupBy = "orgao"
	}

	var out types.SupplierContractsResult
	out.Meta.SupplierID = supplierID
	out.Meta.Limit = limit
	out.Meta.GroupBy = groupBy
	out.Results = []types.SupplierContract{}
	out.Aggregation = []types.SupplierAggregation{}

	if err := s.db.QueryRow(ctx, `
		SELECT COUNT(*) AS contratos,
			COALESCE(SUM(COALESCE(valor_global, valor_inicial, 0)), 0) AS valor_total
		FROM dados_livres_pncp_contrato
		WHERE ni_fornecedor = $1`, supplierID).Scan(&out.Meta.TotalContracts, &out.Meta.TotalValue); err != nil {
		return nil, err
	}

	rows, err := s.db.Query(ctx, `
		SELECT numero_controle_pncp, objeto_contrato, nome_razao_social_fornecedor,
			ni_fornecedor, valor_global, valor_inicial, data_assinatura,
			data_vigencia_inicio, data_vigencia_fim, orgao_cnpj,
			orgao_razao_social, codigo_ibge, uf_sigla
		FROM dados_livres_pncp_contrato
		WHERE ni_fornecedor = $1
		ORDER BY COALESCE(valor_global, valor_inicial, 0) DESC
		LIMIT $2`, supplierID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var item types.SupplierContract
		if err := rows.Scan(
			&item.ControlNumberPNCP, &item.ContractObject, &item.SupplierName,
			&item.SupplierID, &item.GlobalValue, &item.InitialValue,
			&item.SignatureDate, &item.StartDate, &item.EndDate,
			&item.AgencyCNPJ, &item.AgencyName, &item.IBGECode, &item.StateAcronym,
		); err != nil {
			return nil, err
		}
		out.Results = append(out.Results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	aggregation, err := s.aggregateSupplierContracts(ctx, supplierID, groupBy)
	if err != nil {
		return nil, err
	}
	out.Aggregation = aggregation

	return &out, nil
}

func (s *Store) aggregateSupplierContracts(ctx context.Context, supplierID, groupBy string) ([]types.SupplierAggregation, error) {
	var keyColumn, labelColumn string
	switch groupBy {
	case "municipio":
		keyColumn = "codigo_ibge"
		labelColumn = "uf_sigla"
	case "uf":
		keyColumn = "uf_sigla"
		labelColumn = "uf_sigla"
	default:
		keyColumn = "orgao_cnpj"
		labelColumn = "orgao_razao_social"
	}

	rows, err := s.db.Query(ctx, fmt.Sprintf(`
		SELECT %s AS chave, %s AS rotulo,
			COUNT(*) AS contratos,
			SUM(COALESCE(valor_global, valor_inicial, 0)) AS valor_total
		FROM dados_livres_pncp_contrato
		WHERE ni_fornecedor = $1
		GROUP BY %s, %s
		ORDER BY valor_total DESC`, keyColumn, labelColumn, keyColumn, labelColumn), supplierID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.SupplierAggregation{}
	for rows.Next() {
		var item types.SupplierAggregation
		if err := rows.Scan(&item.Key, &item.Label, &item.Contracts, &item.TotalValue); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// -----------------------------------------------------------------------------
// SearchContracts (busca de contratos por escopo)
// -----------------------------------------------------------------------------

// SearchContracts retorna os contratos do escopo informado. O orgao e filtrado
// pelo cnpj (modo --cnpj-orgao); UF, municipio e datas usam os campos
// enriquecidos do proprio contrato.
func (s *Store) SearchContracts(ctx context.Context, filter types.SearchFilter) ([]types.ContractSearch, error) {
	conds := []string{}
	args := []any{}
	pos := 1

	add := func(cond string, value any) {
		conds = append(conds, fmt.Sprintf(cond, pos))
		args = append(args, value)
		pos++
	}

	if filter.State != "" {
		add("uf_sigla = $%d", strings.ToUpper(filter.State))
	}
	if filter.Municipality != "" {
		add("codigo_ibge = $%d", filter.Municipality)
	}
	if filter.AgencyCNPJ != "" {
		add("orgao_cnpj = $%d", strutil.OnlyDigits(filter.AgencyCNPJ))
	}
	if filter.StartPublicationDate != "" {
		add("data_publicacao_pncp >= $%d::date", filter.StartPublicationDate)
	}
	if filter.EndPublicationDate != "" {
		add("data_publicacao_pncp < ($%d::date + INTERVAL '1 day')", filter.EndPublicationDate)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	sqlBase := `SELECT
		numero_controle_pncp, numero_controle_pncp_compra,
		ano_contrato, sequencial_contrato, objeto_contrato,
		ni_fornecedor, nome_razao_social_fornecedor,
		valor_inicial, valor_global, valor_acumulado,
		data_assinatura, data_vigencia_inicio, data_vigencia_fim,
		data_publicacao_pncp, orgao_cnpj, orgao_razao_social,
		codigo_ibge, municipio_nome, uf_sigla
		FROM dados_livres_pncp_contrato` + where +
		` ORDER BY uf_sigla, codigo_ibge, data_publicacao_pncp DESC, numero_controle_pncp`

	rows, err := s.db.Query(ctx, sqlBase, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.ContractSearch{}
	for rows.Next() {
		var item types.ContractSearch
		if err := rows.Scan(
			&item.ControlNumberPNCP, &item.PurchaseControlNumber,
			&item.ContractYear, &item.ContractSequential, &item.ContractObject,
			&item.SupplierID, &item.SupplierName,
			&item.InitialValue, &item.GlobalValue, &item.AccumulatedValue,
			&item.SignatureDate, &item.StartDate, &item.EndDate,
			&item.PublicationDate, &item.AgencyCNPJ, &item.AgencyName,
			&item.IBGECode, &item.MunicipalityName, &item.StateAcronym,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// SearchProcurements (harvest de contratacoes por UF/municipio)
// -----------------------------------------------------------------------------

// SearchProcurements retorna as contratacoes publicadas no escopo (UF,
// municipio, orgao, modalidade, datas) persistidas pelo harvest.
func (s *Store) SearchProcurements(ctx context.Context, filter types.SearchFilter) ([]types.ProcurementSearch, error) {
	conds := []string{}
	args := []any{}
	pos := 1

	add := func(cond string, value any) {
		conds = append(conds, fmt.Sprintf(cond, pos))
		args = append(args, value)
		pos++
	}

	if filter.State != "" {
		add("uf_sigla = $%d", strings.ToUpper(filter.State))
	}
	if filter.Municipality != "" {
		add("codigo_ibge = $%d", filter.Municipality)
	}
	if filter.AgencyCNPJ != "" {
		add("orgao_cnpj = $%d", strutil.OnlyDigits(filter.AgencyCNPJ))
	}
	if filter.Modality > 0 {
		add("modalidade_id = $%d", filter.Modality)
	}
	if filter.StartPublicationDate != "" {
		add("data_publicacao_pncp >= $%d::date", filter.StartPublicationDate)
	}
	if filter.EndPublicationDate != "" {
		add("data_publicacao_pncp < ($%d::date + INTERVAL '1 day')", filter.EndPublicationDate)
	}

	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}

	sqlBase := `SELECT
		numero_controle_pncp, numero_compra, ano_compra, sequencial_compra,
		modalidade_id, modalidade_nome, objeto_compra,
		valor_total_estimado, valor_total_homologado, data_publicacao_pncp,
		orgao_cnpj, orgao_razao_social, codigo_ibge, municipio_nome, uf_sigla
		FROM dados_livres_pncp_contratacao` + where +
		` ORDER BY uf_sigla, codigo_ibge, data_publicacao_pncp DESC, numero_controle_pncp`

	rows, err := s.db.Query(ctx, sqlBase, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []types.ProcurementSearch{}
	for rows.Next() {
		var item types.ProcurementSearch
		if err := rows.Scan(
			&item.ControlNumberPNCP, &item.PurchaseNumber, &item.PurchaseYear,
			&item.PurchaseSequential, &item.ModalityID, &item.ModalityName,
			&item.PurchaseObject, &item.EstimatedTotalValue, &item.ApprovedTotalValue,
			&item.PublicationDate, &item.AgencyCNPJ, &item.AgencyName,
			&item.IBGECode, &item.MunicipalityName, &item.StateAcronym,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
