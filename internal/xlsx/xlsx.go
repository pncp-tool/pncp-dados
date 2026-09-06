// Pacote xlsx implementa storage.Storage gravando os dados em um arquivo Excel
// (XLSX) gerado pela biblioteca tealeg/xlsx. Upsert em planilha e feito de
// forma idempotente pela chave numero_controle_pncp; a cada operacao o
// workbook e reescrito a partir do estado em memoria.
package xlsx

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tealeg/xlsx/v3"

	"github.com/danyele/dados-livres/internal/strutil"
	"github.com/danyele/dados-livres/pncp/types"
)

const (
	sheetContracts    = "contratos"
	sheetProcurements = "contratacoes"
	sheetMeta         = "meta"
)

// contractRow guarda uma linha de contrato como strings; as escritas usam o
// tipo nativo do campo (numerico vira numero na planilha).
type contractRow []string

var contractColumns = []string{
	"numero_controle_pncp", "numero_controle_pncp_compra",
	"ano_contrato", "sequencial_contrato", "objeto_contrato",
	"ni_fornecedor", "tipo_pessoa", "nome_razao_social_fornecedor",
	"ni_fornecedor_sub_contratado", "nome_fornecedor_sub_contratado",
	"valor_inicial", "valor_global", "valor_acumulado",
	"data_assinatura", "data_vigencia_inicio", "data_vigencia_fim",
	"data_publicacao_pncp", "data_atualizacao_global",
	"orgao_cnpj", "orgao_razao_social", "codigo_ibge",
	"municipio_nome", "uf_sigla", "tipo_contrato", "dados_json",
}

// contractFloatColumns sao colunas de contrato persistidas como numero decimal.
var contractFloatColumns = map[int]bool{10: true, 11: true, 12: true}

// contractIntColumns sao colunas de contrato persistidas como numero inteiro.
var contractIntColumns = map[int]bool{2: true, 3: true}

// procurementRow guarda uma linha de contratacao como strings.
type procurementRow []string

var procurementColumns = []string{
	"numero_controle_pncp", "numero_compra",
	"ano_compra", "sequencial_compra",
	"modalidade_id", "modalidade_nome",
	"modo_disputa_id", "modo_disputa_nome",
	"situacao_compra_id", "situacao_compra_nome",
	"objeto_compra", "valor_total_estimado", "valor_total_homologado",
	"data_publicacao_pncp", "data_inclusao", "data_atualizacao",
	"data_atualizacao_global", "data_abertura_proposta", "data_encerramento_proposta",
	"orgao_cnpj", "orgao_razao_social", "codigo_ibge",
	"municipio_nome", "uf_sigla", "unidade_nome",
	"tipo_instrumento_convocatorio_codigo", "tipo_instrumento_convocatorio_nome",
	"srp", "emenda_parlamentar",
	"processo", "link_processo_eletronico", "link_sistema_origem",
	"usuario_nome", "dados_json",
}

// procurementFloatColumns sao colunas de contratacao persistidas como decimal.
var procurementFloatColumns = map[int]bool{11: true, 12: true}

// procurementIntColumns sao colunas de contratacao persistidas como inteiro.
var procurementIntColumns = map[int]bool{2: true, 3: true, 4: true, 6: true, 8: true, 25: true}

type coverageRow []string

var coverageColumns = []string{
	"entidade", "uf", "municipio", "modalidade", "data_inicio", "data_fim",
	"status", "total_registros", "ultima_pagina_ok", "pagina_erro", "atualizado_em",
	"orgao_cnpj",
}

// Store e o backend XLSX.
type Store struct {
	mu sync.Mutex

	path string

	contractsIndex    map[string]int
	contracts         []contractRow
	procurementsIndex map[string]int
	procurements      []procurementRow

	coveragesIndex map[string]int
	coverages      []coverageRow
}

// New abre (ou cria) a planilha no caminho informado.
func New(path string) (*Store, error) {
	s := &Store{
		path:              path,
		contractsIndex:    map[string]int{},
		procurementsIndex: map[string]int{},
		coveragesIndex:    map[string]int{},
		contracts:         []contractRow{},
		procurements:      []procurementRow{},
		coverages:         []coverageRow{},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) load() error {
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return nil
	}
	file, err := xlsx.OpenFile(s.path)
	if err != nil {
		return fmt.Errorf("abrir planilha %s: %w", s.path, err)
	}
	for _, sheet := range file.Sheets {
		rows := [][]string{}
		if err := sheet.ForEachRow(func(row *xlsx.Row) error {
			values := make([]string, 0, 40)
			if err := row.ForEachCell(func(c *xlsx.Cell) error {
				values = append(values, c.String())
				return nil
			}); err != nil {
				return err
			}
			rows = append(rows, values)
			return nil
		}); err != nil {
			return fmt.Errorf("ler planilha %s: %w", sheet.Name, err)
		}
		switch sheet.Name {
		case sheetContracts:
			s.readContracts(rows)
		case sheetProcurements:
			s.readProcurements(rows)
		case sheetMeta:
			s.readCoverages(rows)
		}
	}
	return nil
}

func (s *Store) readContracts(rows [][]string) {
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 1 || row[0] == "" {
			continue
		}
		c := normalizeContract(row)
		s.contractsIndex[c[0]] = len(s.contracts)
		s.contracts = append(s.contracts, c)
	}
}

func (s *Store) readProcurements(rows [][]string) {
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 1 || row[0] == "" {
			continue
		}
		c := normalizeProcurement(row)
		s.procurementsIndex[c[0]] = len(s.procurements)
		s.procurements = append(s.procurements, c)
	}
}

func (s *Store) readCoverages(rows [][]string) {
	for i, row := range rows {
		if i == 0 {
			continue
		}
		if len(row) < 6 || row[0] == "" {
			continue
		}
		c := normalizeCoverage(row)
		s.coveragesIndex[coverageKey(c)] = len(s.coverages)
		s.coverages = append(s.coverages, c)
	}
}

func normalizeContract(row []string) contractRow {
	c := make(contractRow, len(contractColumns))
	for i := range contractColumns {
		if i < len(row) {
			c[i] = strings.TrimSpace(row[i])
		}
	}
	return c
}

func normalizeProcurement(row []string) procurementRow {
	c := make(procurementRow, len(procurementColumns))
	for i := range procurementColumns {
		if i < len(row) {
			c[i] = strings.TrimSpace(row[i])
		}
	}
	return c
}

func normalizeCoverage(row []string) coverageRow {
	c := make(coverageRow, len(coverageColumns))
	for i := range coverageColumns {
		if i < len(row) {
			c[i] = strings.TrimSpace(row[i])
		}
	}
	return c
}

func coverageKey(c coverageRow) string {
	return scopeKey(c[0], c[1], c[2], c[11], c[4], c[5], strutil.MustAtoi(c[3]))
}

func scopeKey(entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) string {
	return entity + "\x1f" + state + "\x1f" + municipality + "\x1f" +
		strconv.Itoa(modality) + "\x1f" + agencyCNPJ + "\x1f" + startDate + "\x1f" + endDate
}

// -----------------------------------------------------------------------------
// contratos
// -----------------------------------------------------------------------------

func (s *Store) UpsertContracts(ctx context.Context, contracts []types.Contract) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, row := range contracts {
		c := contractFromRow(&row)
		if idx, ok := s.contractsIndex[c[0]]; ok {
			s.contracts[idx] = c
		} else {
			s.contractsIndex[c[0]] = len(s.contracts)
			s.contracts = append(s.contracts, c)
		}
	}
	return s.save()
}

func contractFromRow(row *types.Contract) contractRow {
	c := make(contractRow, len(contractColumns))
	c[0] = row.ControlNumberPNCP
	c[1] = text(row.PurchaseControlNumber)
	c[2] = integer(row.ContractYear)
	c[3] = integer(row.ContractSequential)
	c[4] = row.ContractObject
	c[5] = text(row.SupplierID)
	c[6] = text(row.PersonType)
	c[7] = row.SupplierName
	c[8] = text(row.SubcontractorID)
	c[9] = text(row.SubcontractorName)
	c[10] = decimal(row.InitialValue)
	c[11] = decimal(row.GlobalValue)
	c[12] = decimal(row.AccumulatedValue)
	c[13] = timestamp(row.SignatureDate)
	c[14] = timestamp(row.StartDate)
	c[15] = timestamp(row.EndDate)
	c[16] = timestamp(row.PublicationDate)
	c[17] = timestamp(row.UpdateDate)
	c[18] = text(row.AgencyCNPJ)
	c[19] = text(row.AgencyName)
	c[20] = text(row.IBGECode)
	c[21] = text(row.MunicipalityName)
	c[22] = text(row.StateAcronym)
	c[23] = text(row.ContractType)
	c[24] = string(row.RawJSON)
	return c
}

// -----------------------------------------------------------------------------
// contratacoes
// -----------------------------------------------------------------------------

func (s *Store) UpsertProcurements(ctx context.Context, procurements []types.Procurement) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, row := range procurements {
		c := procurementFromRow(&row)
		if idx, ok := s.procurementsIndex[c[0]]; ok {
			s.procurements[idx] = c
		} else {
			s.procurementsIndex[c[0]] = len(s.procurements)
			s.procurements = append(s.procurements, c)
		}
	}
	return s.save()
}

func procurementFromRow(row *types.Procurement) procurementRow {
	c := make(procurementRow, len(procurementColumns))
	c[0] = row.ControlNumberPNCP
	c[1] = text(row.PurchaseNumber)
	c[2] = integer(row.PurchaseYear)
	c[3] = integer(row.PurchaseSequential)
	c[4] = integer(row.ModalityID)
	c[5] = text(row.ModalityName)
	c[6] = integer(row.DisputeModeID)
	c[7] = text(row.DisputeModeName)
	c[8] = integer(row.PurchaseStatusID)
	c[9] = text(row.PurchaseStatusName)
	c[10] = row.PurchaseObject
	c[11] = decimal(row.EstimatedTotalValue)
	c[12] = decimal(row.ApprovedTotalValue)
	c[13] = timestamp(row.PublicationDate)
	c[14] = timestamp(row.InclusionDate)
	c[15] = timestamp(row.UpdateDate)
	c[16] = timestamp(row.GlobalUpdateDate)
	c[17] = timestamp(row.BidOpeningDate)
	c[18] = timestamp(row.BidClosingDate)
	c[19] = text(row.AgencyCNPJ)
	c[20] = text(row.AgencyName)
	c[21] = text(row.IBGECode)
	c[22] = text(row.MunicipalityName)
	c[23] = text(row.StateAcronym)
	c[24] = text(row.UnitName)
	c[25] = integer(row.InstrumentTypeCode)
	c[26] = text(row.InstrumentTypeName)
	c[27] = boolean(row.SRP)
	c[28] = boolean(row.BudgetAmendment)
	c[29] = row.Process
	c[30] = row.ElectronicProcessLink
	c[31] = row.SourceSystemLink
	c[32] = row.UserName
	c[33] = string(row.RawJSON)
	return c
}

// -----------------------------------------------------------------------------
// coberturas
// -----------------------------------------------------------------------------

func (s *Store) upsertCoverage(c coverageRow) {
	key := coverageKey(c)
	if idx, ok := s.coveragesIndex[key]; ok {
		s.coverages[idx] = c
	} else {
		s.coveragesIndex[key] = len(s.coverages)
		s.coverages = append(s.coverages, c)
	}
}

// RecordCoverage marca um escopo com status e total, preservando a ultima
// pagina persistida.
func (s *Store) RecordCoverage(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int, status string, total *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := scopeKey(entity, state, municipality, agencyCNPJ, startDate, endDate, modality)
	var c coverageRow
	if idx, ok := s.coveragesIndex[key]; ok {
		c = append(coverageRow(nil), s.coverages[idx]...)
	} else {
		c = newCoverage(entity, state, municipality, agencyCNPJ, modality, startDate, endDate)
	}
	c[6] = status
	if total != nil {
		c[7] = strconv.Itoa(*total)
	}
	c[9] = ""
	c[10] = time.Now().UTC().Format(time.RFC3339Nano)
	if idx, ok := s.coveragesIndex[key]; ok {
		s.coverages[idx] = c
	} else {
		s.coveragesIndex[coverageKey(c)] = len(s.coverages)
		s.coverages = append(s.coverages, c)
	}
	return s.save()
}

// RecordPageProgress registra o andamento por pagina.
func (s *Store) RecordPageProgress(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, page int, total *int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.coveragesIndex[scopeKey(entity, state, municipality, agencyCNPJ, startDate, endDate, modality)]
	var c coverageRow
	if ok {
		c = append(coverageRow(nil), s.coverages[idx]...)
	} else {
		c = newCoverage(entity, state, municipality, agencyCNPJ, modality, startDate, endDate)
	}
	c[6] = types.StatusPartial
	if previous := strutil.MustAtoi(c[8]); page > previous {
		c[8] = strconv.Itoa(page)
	}
	if c[7] == "" && total != nil {
		c[7] = strconv.Itoa(*total)
	}
	c[10] = time.Now().UTC().Format(time.RFC3339Nano)

	if ok {
		s.coverages[idx] = c
	} else {
		s.coveragesIndex[coverageKey(c)] = len(s.coverages)
		s.coverages = append(s.coverages, c)
	}
	return s.save()
}

// RecordPageFailure marca o escopo com erro e a pagina da falha.
func (s *Store) RecordPageFailure(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, errorPage int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx, ok := s.coveragesIndex[scopeKey(entity, state, municipality, agencyCNPJ, startDate, endDate, modality)]
	var c coverageRow
	if ok {
		c = append(coverageRow(nil), s.coverages[idx]...)
	} else {
		c = newCoverage(entity, state, municipality, agencyCNPJ, modality, startDate, endDate)
	}
	c[6] = types.StatusError
	c[9] = strconv.Itoa(errorPage)
	c[10] = time.Now().UTC().Format(time.RFC3339Nano)

	if ok {
		s.coverages[idx] = c
	} else {
		s.coveragesIndex[coverageKey(c)] = len(s.coverages)
		s.coverages = append(s.coverages, c)
	}
	return s.save()
}

func newCoverage(entity, state, municipality, agencyCNPJ string, modality int, startDate, endDate string) coverageRow {
	c := make(coverageRow, len(coverageColumns))
	c[0] = entity
	c[1] = state
	c[2] = municipality
	c[3] = strconv.Itoa(modality)
	c[4] = startDate
	c[5] = endDate
	c[8] = "0"
	c[11] = agencyCNPJ
	return c
}

// -----------------------------------------------------------------------------
// leitura/estado
// -----------------------------------------------------------------------------

// LastPageOK devolve a ultima pagina persistida do escopo.
func (s *Store) LastPageOK(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.coveragesIndex[scopeKey(entity, state, municipality, agencyCNPJ, startDate, endDate, modality)]
	if !ok {
		return 0, nil
	}
	return strutil.MustAtoi(s.coverages[idx][8]), nil
}

// CoverageCompleted informa se o escopo ja foi concluido.
func (s *Store) CoverageCompleted(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.coveragesIndex[scopeKey(entity, state, municipality, agencyCNPJ, startDate, endDate, modality)]
	if !ok {
		return false, nil
	}
	return s.coverages[idx][6] == types.StatusCompleted, nil
}

// ListCoverages le as coberturas em memoria.
func (s *Store) ListCoverages(ctx context.Context) ([]types.Coverage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]types.Coverage, 0, len(s.coverages))
	for _, c := range s.coverages {
		out = append(out, coverageToType(c))
	}
	return out, nil
}

func coverageToType(c coverageRow) types.Coverage {
	return types.Coverage{
		Entity:       c[0],
		State:        c[1],
		Municipality: c[2],
		Modality:     strutil.MustAtoi(c[3]),
		StartDate:    c[4],
		EndDate:      c[5],
		Status:       c[6],
		TotalRecords: ptrInt(c[7]),
		LastPageOK:   strutil.MustAtoi(c[8]),
		ErrorPage:    ptrInt(c[9]),
		UpdatedAt:    ptrTime(c[10]),
		AgencyCNPJ:   c[11],
	}
}

func ptrInt(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	n := strutil.MustAtoi(s)
	return &n
}

func ptrTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return &t
	}
	return nil
}

// CountRecords conta os contratos em memoria.
func (s *Store) CountRecords(ctx context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.contracts), nil
}

// Status monta o estado agregado a partir das coberturas.
func (s *Store) Status(ctx context.Context) (*types.IndexStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := &types.IndexStatus{
		Exists:    len(s.contracts) > 0 || len(s.procurements) > 0 || len(s.coverages) > 0,
		Records:   len(s.contracts) + len(s.procurements),
		Coverages: []types.Coverage{},
	}
	for _, c := range s.coverages {
		coverage := coverageToType(c)
		if coverage.Status != types.StatusCompleted {
			continue
		}
		if status.Window.StartDate == "" || coverage.StartDate < status.Window.StartDate {
			status.Window.StartDate = coverage.StartDate
		}
		if coverage.EndDate > status.Window.EndDate {
			status.Window.EndDate = coverage.EndDate
		}
		if coverage.UpdatedAt != nil &&
			(status.UpdatedAt == nil || coverage.UpdatedAt.After(*status.UpdatedAt)) {
			status.UpdatedAt = coverage.UpdatedAt
		}
		status.Coverages = append(status.Coverages, coverage)
	}
	return status, nil
}

// -----------------------------------------------------------------------------
// gravacao do workbook
// -----------------------------------------------------------------------------

func (s *Store) save() error {
	file := xlsx.NewFile()

	shContracts, err := file.AddSheet(sheetContracts)
	if err != nil {
		return err
	}
	writeRows(shContracts, contractColumns, s.contracts, contractFloatColumns, contractIntColumns)

	shProcurements, err := file.AddSheet(sheetProcurements)
	if err != nil {
		return err
	}
	writeRows(shProcurements, procurementColumns, s.procurements, procurementFloatColumns, procurementIntColumns)

	shMeta, err := file.AddSheet(sheetMeta)
	if err != nil {
		return err
	}
	header := shMeta.AddRow()
	for _, name := range coverageColumns {
		header.AddCell().SetString(name)
	}
	for _, c := range s.coverages {
		row := shMeta.AddRow()
		for i, value := range c {
			if i == 3 || i == 7 || i == 8 || i == 9 {
				if value != "" {
					if v, e := strconv.Atoi(value); e == nil {
						row.AddCell().SetInt(v)
						continue
					}
				}
			}
			row.AddCell().SetString(value)
		}
	}

	tmp := s.path + ".tmp"
	if dir := filepath.Dir(s.path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("criar diretorio %q: %w", dir, err)
		}
	}
	if err := file.Save(tmp); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func writeRows[T ~[]string](sheet *xlsx.Sheet, columns []string, rows []T, floatColumns, intColumns map[int]bool) {
	header := sheet.AddRow()
	for _, name := range columns {
		header.AddCell().SetString(name)
	}
	for _, c := range rows {
		row := sheet.AddRow()
		for i, value := range c {
			if floatColumns[i] {
				if v, e := strconv.ParseFloat(value, 64); e == nil {
					row.AddCell().SetFloat(v)
					continue
				}
			}
			if intColumns[i] {
				if v, e := strconv.Atoi(value); e == nil {
					row.AddCell().SetInt(v)
					continue
				}
			}
			row.AddCell().SetString(value)
		}
	}
}

func text(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func integer(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func decimal(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func timestamp(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func boolean(value *bool) string {
	if value == nil {
		return ""
	}
	if *value {
		return "true"
	}
	return "false"
}
