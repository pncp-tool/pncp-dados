// Pacote harvest orquestra o harvest do indice offline do PNCP (paginar a API
// de Consulta v1, mapear e persistir) e expoe os casos de uso de busca.
package harvest

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/danyele/dados-livres/internal/client"
	"github.com/danyele/dados-livres/internal/logger"
	"github.com/danyele/dados-livres/internal/parse"
	"github.com/danyele/dados-livres/internal/progress"
	"github.com/danyele/dados-livres/internal/strutil"
	"github.com/danyele/dados-livres/pncp/types"
	"github.com/danyele/dados-livres/storage"
)

// ErrEmptyScope e retornado quando o escopo nao define nenhuma dimensao de
// recorte (sem UFs, sem municipios e sem CNPJs de orgao).
var ErrEmptyScope = errors.New("escopo vazio: informe UFs/municipios (contratacoes) ou --cnpj-orgao (contratos)")

// procurementModalities cobre as modalidades aceitas pela API de contratacoes
// publicadas (1..15).
var procurementModalities = []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}

// Config agrupa as configuracoes do harvest.
type Config struct {
	// MaxPages limita o numero de paginas por escopo/cobertura. 0 = sem limite.
	MaxPages int
}

// Harvester implementa o fluxo de harvest e as consultas do PNCP offline.
type Harvester struct {
	log      *logger.Logger
	client   *client.Client
	store    storage.Storage
	progress *progress.HarvestProgress
	maxPages int
}

// New cria o Harvester de harvest/busca do PNCP.
func New(client *client.Client, store storage.Storage, p *progress.HarvestProgress, cfg Config) *Harvester {
	return &Harvester{
		log:      logger.New("dados-livres: harvest"),
		client:   client,
		store:    store,
		progress: p,
		maxPages: cfg.MaxPages,
	}
}

// harvestUnit descreve uma unidade de varredura: um orgao (contratos) ou uma
// combinacao de UF/municipio/modalidade (contratacoes).
type harvestUnit struct {
	entity       string
	state        string
	municipality string
	agencyCNPJ   string
	modality     int
	endpoint     string
	params       map[string]string
	pageSize     int
	maxPages     int
}

// pageResult e o desfecho de uma varredura de pagina.
type pageResult struct {
	Persisted int
	Expected  int
	ErrorPage int
	Completed bool
}

// -----------------------------------------------------------------------------
// Harvest
// -----------------------------------------------------------------------------

// Harvest pagina a API de Consulta v1 com filtro server-side e faz upsert na
// persistencia, publicando o andamento no HarvestProgress interno. O recorte
// do escopo decide o canal:
//
//	-- a lista de AgencyCNPJ harvesta contratos por orgao (/contratos?cnpjOrgao);
//	-- States/municipalities harvestam contratacoes publicadas por UF e
//	  municipio (/contratacoes/publicacao?uf=&codigoMunicipioIbge=), cobrindo
//	  todas as modalidades;
//	-- escopo sem dimensao e recusado com ErrEmptyScope.
//
// Escopos ja concluidos sao pulados (dedup); falhas gravam status 'error'.
func (h *Harvester) Harvest(ctx context.Context, scope types.Scope) (*types.HarvestSummary, error) {
	return h.harvest(ctx, scope, nil)
}

// HarvestWithProgress pagina o escopo publicando o andamento em um
// HarvestProgress dedicado (ex.: por job). Se nil, usa o interno.
func (h *Harvester) HarvestWithProgress(ctx context.Context, scope types.Scope, p *progress.HarvestProgress) (*types.HarvestSummary, error) {
	return h.harvest(ctx, scope, p)
}

func (h *Harvester) harvest(ctx context.Context, scope types.Scope, customProgress *progress.HarvestProgress) (*types.HarvestSummary, error) {
	p := h.progress
	if customProgress != nil {
		p = customProgress
	}

	window := resolveWindow(scope)
	summary := &types.HarvestSummary{
		Window:    window,
		States:    resolveStates(scope.States),
		UpdatedAt: time.Now().UTC(),
	}

	maxPages := scope.MaxPages
	if maxPages <= 0 {
		maxPages = h.maxPages
	}

	units, err := buildUnits(scope, maxPages)
	if err != nil {
		return summary, err
	}

	for _, unit := range units {
		if err := h.harvestUnit(ctx, unit, summary, p); err != nil {
			return summary, err
		}
	}
	summary.TotalRecords = summary.Inserted.Contracts + summary.Inserted.Procurements
	return summary, nil
}

// buildUnits converte o escopo na lista de unidades de varredura, reagindo ao
// canal usado pela API. Escopo sem dimensao devolve ErrEmptyScope.
func buildUnits(scope types.Scope, maxPages int) ([]harvestUnit, error) {
	if agencies := resolveAgencies(scope.AgencyCNPJ); len(agencies) > 0 {
		units := make([]harvestUnit, 0, len(agencies))
		for _, cnpj := range agencies {
			units = append(units, harvestUnit{
				entity:     types.EntityContracts,
				agencyCNPJ: cnpj,
				endpoint:   client.ContractsEndpoint,
				pageSize:   pageSize(scope.PageSize, types.MaxContractsPageSize),
				maxPages:   maxPages,
				params:     map[string]string{"cnpjOrgao": cnpj},
			})
		}
		return units, nil
	}

	states := resolveStates(scope.States)
	municipalities := resolveMunicipalities(scope.Municipalities)
	if len(states) == 0 && len(municipalities) == 0 {
		return nil, ErrEmptyScope
	}

	stateLoop := states
	if len(stateLoop) == 0 {
		stateLoop = []string{""}
	}
	municipalityLoop := municipalities
	if len(municipalityLoop) == 0 {
		municipalityLoop = []string{""}
	}

	units := make([]harvestUnit, 0)
	for _, state := range stateLoop {
		for _, municipality := range municipalityLoop {
			for _, modality := range procurementModalities {
				params := map[string]string{
					"codigoModalidadeContratacao": strconv.Itoa(modality),
				}
				if state != "" {
					params["uf"] = state
				}
				if municipality != "" {
					params["codigoMunicipioIbge"] = municipality
				}
				units = append(units, harvestUnit{
					entity:       types.EntityProcurements,
					state:        state,
					municipality: municipality,
					modality:     modality,
					endpoint:     client.ProcurementsEndpoint,
					pageSize:     pageSize(scope.PageSize, types.MaxProcurementsPageSize),
					maxPages:     maxPages,
					params:       params,
				})
			}
		}
	}
	return units, nil
}

// harvestUnit pagina uma unidade, persistindo os registros e registrando a
// cobertura. Escopos ja concluidos sao pulados; falhas gravam status 'error'.
func (h *Harvester) harvestUnit(ctx context.Context, unit harvestUnit, summary *types.HarvestSummary, p *progress.HarvestProgress) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.SetEntity(unit.entity)
	p.SetState(unit.state)

	skipped, err := h.skipIfCovered(ctx, unit, summary)
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}

	params := pageParams(summary.Window, unit)
	start := 1
	lastPage, err := h.store.LastPageOK(ctx, unit.entity, unit.state, unit.municipality, unit.agencyCNPJ, summary.Window.StartDate, summary.Window.EndDate, unit.modality)
	if err != nil {
		return err
	}
	if lastPage > 0 {
		start = lastPage + 1
		summary.ResumedFrom = lastPage
		h.log.Info("retomando escopo parcial", "entidade", unit.entity, "uf", unit.state, "ultimaPaginaOK", lastPage, "inicio", start)
	}

	result, err := h.paginate(ctx, unit, params, start, p)
	if err != nil {
		h.recordFailure(ctx, unit, summary.Window, result.ErrorPage)
		return err
	}
	addInserted(&summary.Inserted, unit.entity, result.Persisted)
	if err := h.recordCoverage(ctx, unit, summary.Window, result.Expected, result.Completed); err != nil {
		h.recordFailure(ctx, unit, summary.Window, result.ErrorPage)
		return err
	}
	return nil
}

// pageParams monta o mapa de filtros de uma requisicao: a janela de datas mais
// os filtros proprios da unidade.
func pageParams(window types.Window, unit harvestUnit) map[string]string {
	params := map[string]string{
		"dataInicial": window.StartDate,
		"dataFinal":   window.EndDate,
	}
	for k, v := range unit.params {
		if v != "" {
			params[k] = v
		}
	}
	return params
}

// skipIfCovered verifica se o escopo ja foi concluido; em caso positivo
// incrementa o contador de pulados e retorna true (dedup).
func (h *Harvester) skipIfCovered(ctx context.Context, unit harvestUnit, summary *types.HarvestSummary) (bool, error) {
	exists, err := h.store.CoverageCompleted(ctx, unit.entity, unit.state, unit.municipality, unit.agencyCNPJ, summary.Window.StartDate, summary.Window.EndDate, unit.modality)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}
	h.log.Info("escopo ja concluido, pulando", "entidade", unit.entity, "uf", unit.state, "municipio", unit.municipality, "orgao", unit.agencyCNPJ, "modalidade", unit.modality, "dataInicial", summary.Window.StartDate, "dataFinal", summary.Window.EndDate)
	summary.Skipped++
	return true, nil
}

// recordCoverage registra o escopo como concluido (totalRegistros esperado
// informado pela API) ou parcial quando o harvest parou por limite de paginas
// (--max-paginas), mantendo o escopo retomavel de ultima_pagina_ok + 1.
func (h *Harvester) recordCoverage(ctx context.Context, unit harvestUnit, window types.Window, total int, completed bool) error {
	var totalPtr *int
	if total > 0 {
		totalPtr = &total
	}
	status := types.StatusCompleted
	if !completed {
		status = types.StatusPartial
	}
	return h.store.RecordCoverage(ctx, unit.entity, unit.state, unit.municipality, unit.agencyCNPJ, window.StartDate, window.EndDate, unit.modality, status, totalPtr)
}

// recordFailure marca o escopo com status 'error' e registra a pagina da
// falha (best-effort), permitindo retry retomando de ultima_pagina_ok + 1.
func (h *Harvester) recordFailure(ctx context.Context, unit harvestUnit, window types.Window, errorPage int) {
	if err := h.store.RecordPageFailure(ctx, unit.entity, unit.state, unit.municipality, unit.agencyCNPJ, window.StartDate, window.EndDate, unit.modality, errorPage); err != nil {
		h.log.Error("falha ao registrar cobertura com erro", "entidade", unit.entity, "error", err)
	}
}

// paginate percorre as paginas de um endpoint ate esgotar, mapeando e
// persistindo cada pagina. Retorna o total persistido, o totalRegistros
// esperado (primeira pagina), a pagina em que ocorreu erro e se a varredura
// esgotou todas as paginas. Quando maxPages interrompe o loop, Completed=false
// (cobertura deve ser 'parcial').
func (h *Harvester) paginate(ctx context.Context, unit harvestUnit, params map[string]string, start int, p *progress.HarvestProgress) (pageResult, error) {
	page := start
	if page < 1 {
		page = 1
	}
	completed := true
	var result pageResult

	for {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if unit.maxPages > 0 && page > unit.maxPages {
			completed = false
			break
		}
		p.CurrentPage.Store(int64(page))

		env, err := h.client.Paginate(ctx, unit.endpoint, params, page, unit.pageSize)
		if err != nil {
			p.Errors.Add(1)
			h.log.Error("falha ao paginar", "entidade", unit.entity, "pagina", page, "error", err)
			result.ErrorPage = page
			return result, err
		}
		p.Pages.Add(1)

		if page == 1 {
			result.Expected = env.TotalRecords
		}
		if len(env.Data) == 0 {
			break
		}

		n, err := h.mapAndPersist(ctx, unit.entity, env.Data, p)
		if err != nil {
			p.Errors.Add(1)
			h.log.Error("falha ao persistir", "entidade", unit.entity, "pagina", page, "error", err)
			result.ErrorPage = page
			return result, err
		}
		result.Persisted += n

		var totalPtr *int
		if result.Expected > 0 {
			totalPtr = &result.Expected
		}
		if err := h.store.RecordPageProgress(ctx, unit.entity, unit.state, unit.municipality, unit.agencyCNPJ, params["dataInicial"], params["dataFinal"], unit.modality, page, totalPtr); err != nil {
			p.Errors.Add(1)
			h.log.Error("falha ao registrar andamento", "entidade", unit.entity, "pagina", page, "error", err)
			result.ErrorPage = page
			return result, err
		}

		if env.Empty || env.RemainingPages == 0 || (env.TotalPages > 0 && page >= env.TotalPages) {
			break
		}
		page++
	}

	result.Completed = completed
	return result, nil
}

// mapAndPersist mapeia os registros crus e faz upsert por entidade.
func (h *Harvester) mapAndPersist(ctx context.Context, entity string, records []map[string]any, p *progress.HarvestProgress) (int, error) {
	switch entity {
	case types.EntityContracts:
		contracts := make([]types.Contract, 0, len(records))
		for _, raw := range records {
			if contract := parse.ToContract(raw); contract != nil {
				contracts = append(contracts, *contract)
			}
		}
		if len(contracts) == 0 {
			return 0, nil
		}
		if err := h.store.UpsertContracts(ctx, contracts); err != nil {
			return 0, err
		}
		p.Contracts.Add(int64(len(contracts)))
		return len(contracts), nil
	case types.EntityProcurements:
		procurements := make([]types.Procurement, 0, len(records))
		for _, raw := range records {
			if procurement := parse.ToProcurement(raw); procurement != nil {
				procurements = append(procurements, *procurement)
			}
		}
		if len(procurements) == 0 {
			return 0, nil
		}
		if err := h.store.UpsertProcurements(ctx, procurements); err != nil {
			return 0, err
		}
		p.Procurements.Add(int64(len(procurements)))
		return len(procurements), nil
	default:
		return 0, nil
	}
}

// addInserted soma os registros persistidos ao resumo, por entidade.
func addInserted(inserted *types.Inserted, entity string, count int) {
	switch entity {
	case types.EntityContracts:
		inserted.Contracts += count
	case types.EntityProcurements:
		inserted.Procurements += count
	}
}

// -----------------------------------------------------------------------------
// Consultas
// -----------------------------------------------------------------------------

// Searcher devolve o backend de consultas indexadas, se houver.
func (h *Harvester) Searcher() storage.Searcher {
	if s, ok := h.store.(storage.Searcher); ok {
		return s
	}
	return nil
}

// Status devolve o estado agregado do indice.
func (h *Harvester) Status(ctx context.Context) (*types.IndexStatus, error) {
	return h.store.Status(ctx)
}

// ListCoverages devolve as coberturas registradas.
func (h *Harvester) ListCoverages(ctx context.Context) ([]types.Coverage, error) {
	return h.store.ListCoverages(ctx)
}

// CountRecords conta os contratos persistidos.
func (h *Harvester) CountRecords(ctx context.Context) (int, error) {
	return h.store.CountRecords(ctx)
}

// ProgressEvent devolve o andamento atual do harvest.
func (h *Harvester) ProgressEvent() types.ProgressEvent {
	return h.progress.Event()
}

// SuppliersByName busca fornecedores por razao social (requer Postgres).
func (h *Harvester) SuppliersByName(ctx context.Context, name string, limit int) ([]types.Supplier, error) {
	s := h.Searcher()
	if s == nil {
		return nil, storage.ErrSearchNotSupported
	}
	return s.SuppliersByName(ctx, name, limit)
}

// ContractsBySupplier lista os contratos de um fornecedor (requer Postgres).
func (h *Harvester) ContractsBySupplier(ctx context.Context, supplierID, groupBy string, limit int) (*types.SupplierContractsResult, error) {
	s := h.Searcher()
	if s == nil {
		return nil, storage.ErrSearchNotSupported
	}
	return s.ContractsBySupplier(ctx, supplierID, groupBy, limit)
}

// SearchContracts retorna os contratos publicados no escopo (requer Postgres).
func (h *Harvester) SearchContracts(ctx context.Context, filter types.SearchFilter) ([]types.ContractSearch, error) {
	s := h.Searcher()
	if s == nil {
		return nil, storage.ErrSearchNotSupported
	}
	return s.SearchContracts(ctx, filter)
}

// SearchProcurements retorna as contratacoes publicadas no escopo (requer
// Postgres).
func (h *Harvester) SearchProcurements(ctx context.Context, filter types.SearchFilter) ([]types.ProcurementSearch, error) {
	s := h.Searcher()
	if s == nil {
		return nil, storage.ErrSearchNotSupported
	}
	return s.SearchProcurements(ctx, filter)
}

// -----------------------------------------------------------------------------
// Resolucao de escopo
// -----------------------------------------------------------------------------

// resolveWindow resolve a janela de datas: datas explicitas > mes (YYYY-MM) >
// anos > ano corrente (1o de janeiro ate hoje).
func resolveWindow(scope types.Scope) types.Window {
	if isValidDate(scope.StartDate) && isValidDate(scope.EndDate) {
		return types.Window{StartDate: scope.StartDate, EndDate: scope.EndDate}
	}

	now := time.Now()
	currentYear := now.Year()

	if month := strings.TrimSpace(scope.Month); isValidMonth(month) {
		year := strutil.MustAtoi(month[0:4])
		monthNum := strutil.MustAtoi(month[5:7])
		start := time.Date(year, time.Month(monthNum), 1, 0, 0, 0, 0, time.UTC)
		end := start.AddDate(0, 1, -1)
		return types.Window{
			StartDate: start.Format("20060102"),
			EndDate:   end.Format("20060102"),
		}
	}

	if len(scope.Years) > 0 {
		minor := scope.Years[0]
		major := scope.Years[0]
		for _, year := range scope.Years {
			if year < minor {
				minor = year
			}
			if year > major {
				major = year
			}
		}
		endDate := fmt.Sprintf("%04d1231", major)
		if major >= currentYear {
			endDate = now.Format("20060102")
		}
		return types.Window{
			StartDate: fmt.Sprintf("%04d0101", minor),
			EndDate:   endDate,
		}
	}

	return types.Window{
		StartDate: fmt.Sprintf("%04d0101", currentYear),
		EndDate:   now.Format("20060102"),
	}
}

// clean deduplica itens aplicando uma normalizacao; itens invalidos sao
// descartados. A ordem de aparicao e preservada.
func clean(items []string, normalize func(string) (string, bool)) []string {
	out := make([]string, 0, len(items))
	seen := map[string]bool{}
	for _, item := range items {
		value, ok := normalize(item)
		if !ok || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

// resolveStates normaliza o recorte de UFs (vazio = sem filtro).
func resolveStates(states []string) []string {
	return clean(states, func(s string) (string, bool) {
		state := strings.ToUpper(strings.TrimSpace(s))
		return state, len(state) == 2
	})
}

// resolveMunicipalities normaliza o recorte de municipios (codigo IBGE de 7
// digitos). Vazio = sem filtro.
func resolveMunicipalities(codes []string) []string {
	return clean(codes, func(s string) (string, bool) {
		code := strings.TrimSpace(s)
		return code, len(code) == 7 && strutil.OnlyDigits(code) == code
	})
}

// resolveAgencies normaliza o recorte de CNPJs de orgao (apenas digitos).
func resolveAgencies(cnpjs []string) []string {
	return clean(cnpjs, func(s string) (string, bool) {
		digits := strutil.OnlyDigits(s)
		return digits, len(digits) > 0
	})
}

// pageSize limita o tamanho de pagina ao intervalo aceito pela API.
func pageSize(size, max int) int {
	if size <= 0 {
		return max
	}
	if size < types.MinPageSize {
		return types.MinPageSize
	}
	if size > max {
		return max
	}
	return size
}

func isValidDate(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 8 {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isValidMonth(value string) bool {
	if len(value) != 7 || value[4] != '-' {
		return false
	}
	year := strutil.MustAtoi(value[0:4])
	month := strutil.MustAtoi(value[5:7])
	return year >= 2000 && year <= 2100 && month >= 1 && month <= 12
}
