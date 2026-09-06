package harvest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"testing"

	"github.com/danyele/dados-livres/internal/client"
	"github.com/danyele/dados-livres/internal/progress"
	"github.com/danyele/dados-livres/pncp/types"
	"github.com/danyele/dados-livres/storage"
)

// fakeStore e um storage.Storage em memoria para isolar o fluxo de harvest.
type fakeStore struct {
	inMemory struct {
		coverageStatus string
		coverageTotal  *int
		lastPageOK     int
	}
	upsertContracts    int
	upsertProcurements int
	coverages          []coverageRecord
}

type coverageRecord struct {
	entity, state, municipality, agencyCNPJ string
	modality                                int
	status                                  string
}

func (s *fakeStore) UpsertContracts(ctx context.Context, contracts []types.Contract) error {
	s.upsertContracts += len(contracts)
	return nil
}

func (s *fakeStore) UpsertProcurements(ctx context.Context, procurements []types.Procurement) error {
	s.upsertProcurements += len(procurements)
	return nil
}

func (s *fakeStore) RecordCoverage(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int, status string, total *int) error {
	s.inMemory.coverageStatus = status
	s.coverages = append(s.coverages, coverageRecord{
		entity: entity, state: state, municipality: municipality, agencyCNPJ: agencyCNPJ,
		modality: modality, status: status,
	})
	// Espelha o backend real: total nil preserva o valor ja conhecido.
	if total != nil {
		s.inMemory.coverageTotal = total
	}
	return nil
}

func (s *fakeStore) RecordPageProgress(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, page int, total *int) error {
	return nil
}

func (s *fakeStore) RecordPageFailure(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality, errorPage int) error {
	return nil
}

func (s *fakeStore) LastPageOK(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (int, error) {
	return s.inMemory.lastPageOK, nil
}

func (s *fakeStore) CoverageCompleted(ctx context.Context, entity, state, municipality, agencyCNPJ, startDate, endDate string, modality int) (bool, error) {
	return false, nil
}

func (s *fakeStore) ListCoverages(ctx context.Context) ([]types.Coverage, error) {
	return nil, nil
}

func (s *fakeStore) CountRecords(ctx context.Context) (int, error) {
	return 0, nil
}

func (s *fakeStore) Status(ctx context.Context) (*types.IndexStatus, error) {
	return &types.IndexStatus{}, nil
}

var _ storage.Storage = (*fakeStore)(nil)

// serverContracts devolve um fake do endpoint /contratos com totalPaginas
// paginas de 1 contrato cada. checkRequest recursa a consulta para validacao.
func serverContracts(t *testing.T, totalPages int, check func(path string, q url.Values)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if check != nil {
			check(req.URL.Path, req.URL.Query())
		}
		page := 0
		fmt.Sscanf(req.URL.Query().Get("pagina"), "%d", &page)
		if page < 1 || page > totalPages {
			json.NewEncoder(w).Encode(client.Envelope{Empty: true, TotalPages: totalPages})
			return
		}
		json.NewEncoder(w).Encode(client.Envelope{
			TotalPages:     totalPages,
			TotalRecords:   totalPages,
			PageNumber:     page,
			RemainingPages: totalPages - page,
			Data: []map[string]any{{
				"numeroControlePNCP": fmt.Sprintf("00000000000000-1-%05d/2026", page),
				"orgaoEntidade":      map[string]any{"cnpj": "01409580000138", "razaoSocial": "ESTADO DE GOIAS"},
				"unidadeOrgao": map[string]any{
					"ufSigla":       "GO",
					"codigoIbge":    "5208707",
					"municipioNome": "Goiania",
				},
			}},
		})
	}))
}

func TestPaginateMaxPagesMarksIncomplete(t *testing.T) {
	srv := serverContracts(t, 5, nil)
	defer srv.Close()

	store := &fakeStore{}
	h := New(client.New(srv.URL, 1, 0), store, progress.NewHarvestProgress(), Config{})

	unit := harvestUnit{
		entity:   types.EntityContracts,
		endpoint: client.ContractsEndpoint,
		pageSize: 50,
		maxPages: 3,
		params:   map[string]string{"cnpjOrgao": "01409580000138"},
	}
	window := types.Window{StartDate: "20260101", EndDate: "20260102"}

	result, err := h.paginate(context.Background(), unit, pageParams(window, unit), 1, progress.NewHarvestProgress())
	if err != nil {
		t.Fatalf("paginate: %v", err)
	}
	if result.ErrorPage != 0 {
		t.Fatalf("ErrorPage = %d", result.ErrorPage)
	}
	if result.Expected != 5 {
		t.Errorf("Expected = %d (queria 5)", result.Expected)
	}
	if result.Persisted != 3 {
		t.Errorf("Persisted = %d (queria 3)", result.Persisted)
	}
	if result.Completed {
		t.Error("varredura interrompida por maxPages deve retornar Completed=false")
	}

	// Sem limite, a varredura esgota as 5 paginas e Completed=true.
	unit.maxPages = 0
	result, err = h.paginate(context.Background(), unit, pageParams(window, unit), 1, progress.NewHarvestProgress())
	if err != nil {
		t.Fatalf("paginate sem limite: %v", err)
	}
	if result.Persisted != 5 {
		t.Errorf("Persisted sem limite = %d (queria 5)", result.Persisted)
	}
	if !result.Completed {
		t.Error("varredura exaurida deve retornar Completed=true")
	}
}

func TestHarvestAgencyCNPJUsesServerSideFilter(t *testing.T) {
	// Modo --cnpj-orgao: o harvest aciona /contratos passando cnpjOrgao
	// (o unico filtro aceito pelo endpoint) e registra cobertura por orgao.
	srv := serverContracts(t, 3, func(path string, q url.Values) {
		if path != "/contratos" {
			t.Errorf("path = %q (queria /contratos)", path)
		}
		if cnpj := q.Get("cnpjOrgao"); cnpj != "01409580000138" {
			t.Errorf("cnpjOrgao = %q (queria 01409580000138)", cnpj)
		}
	})
	defer srv.Close()

	store := &fakeStore{}
	h := New(client.New(srv.URL, 1, 0), store, progress.NewHarvestProgress(), Config{})

	summary, err := h.Harvest(context.Background(), types.Scope{
		StartDate:  "20260101",
		EndDate:    "20260102",
		AgencyCNPJ: []string{"01409580000138"},
	})
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if summary.Inserted.Contracts != 3 {
		t.Errorf("Inserted.Contracts = %d (queria 3)", summary.Inserted.Contracts)
	}
	if len(store.coverages) != 1 {
		t.Fatalf("coberturas = %d (queria 1)", len(store.coverages))
	}
	if store.coverages[0].agencyCNPJ != "01409580000138" || store.coverages[0].entity != types.EntityContracts {
		t.Errorf("cobertura = %+v (queria orgao 01409580000138 contratos)", store.coverages[0])
	}
	if store.coverages[0].status != types.StatusCompleted {
		t.Errorf("status da cobertura = %q (queria %q)", store.coverages[0].status, types.StatusCompleted)
	}
	if store.inMemory.coverageTotal == nil || *store.inMemory.coverageTotal != 3 {
		t.Errorf("TotalRecords da cobertura = %v (queria 3)", store.inMemory.coverageTotal)
	}
}

func TestHarvestProcurementsUsesServerSideFilter(t *testing.T) {
	// Modo UF/municipio: o harvest aciona /contratacoes/publicacao cobrindo as
	// 15 modalidades e passa uf + codigoMunicipioIbge como filtro server-side
	// (nenhum filtro local). Cada requisicao precisa casar o escopo.
	seen := map[int]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path != "/contratacoes/publicacao" {
			t.Errorf("path = %q (queria /contratacoes/publicacao)", req.URL.Path)
		}
		q := req.URL.Query()
		if q.Get("uf") != "GO" {
			t.Errorf("uf = %q (queria GO)", q.Get("uf"))
		}
		if q.Get("codigoMunicipioIbge") != "5208707" {
			t.Errorf("codigoMunicipioIbge = %q (queria 5208707)", q.Get("codigoMunicipioIbge"))
		}
		modality := 0
		fmt.Sscanf(q.Get("codigoModalidadeContratacao"), "%d", &modality)
		if modality < 1 || modality > 15 {
			t.Errorf("modalidadeId = %d (fora de 1..15)", modality)
		}
		seen[modality] = true
		json.NewEncoder(w).Encode(client.Envelope{
			TotalPages:     1,
			TotalRecords:   1,
			PageNumber:     1,
			RemainingPages: 0,
			Data: []map[string]any{{
				"numeroControlePNCP": fmt.Sprintf("00000000000000-0-%02d/2026", modality),
				"modalidadeId":       modality,
				"orgaoEntidade":      map[string]any{"cnpj": "01409580000138", "razaoSocial": "ESTADO DE GOIAS"},
				"unidadeOrgao": map[string]any{
					"ufSigla":       "GO",
					"codigoIbge":    "5208707",
					"municipioNome": "Goiania",
				},
			}},
		})
	}))
	defer srv.Close()

	store := &fakeStore{}
	h := New(client.New(srv.URL, 1, 0), store, progress.NewHarvestProgress(), Config{})

	summary, err := h.Harvest(context.Background(), types.Scope{
		StartDate:      "20260101",
		EndDate:        "20260102",
		States:         []string{"GO"},
		Municipalities: []string{"5208707"},
	})
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if len(seen) != 15 {
		t.Errorf("modalidades consultadas = %d (queria 15)", len(seen))
	}
	if summary.Inserted.Procurements != 15 {
		t.Errorf("Inserted.Procurements = %d (queria 15)", summary.Inserted.Procurements)
	}
	if store.upsertContracts != 0 {
		t.Errorf("upserts de contratos = %d (queria 0)", store.upsertContracts)
	}
	if len(store.coverages) != 15 {
		t.Fatalf("coberturas = %d (queria 15, uma por modalidade)", len(store.coverages))
	}
	modalities := []int{}
	for _, c := range store.coverages {
		if c.entity != types.EntityProcurements || c.state != "GO" || c.municipality != "5208707" {
			t.Errorf("cobertura = %+v (queria contratacoes GO/5208707)", c)
		}
		modalities = append(modalities, c.modality)
	}
	sort.Ints(modalities)
	for i, m := range modalities {
		if m != i+1 {
			t.Errorf("modalidades cobertas = %v (queria 1..15)", modalities)
			break
		}
	}
}

func TestHarvestAgencyCNPJWithMaxPagesMarksPartial(t *testing.T) {
	srv := serverContracts(t, 5, nil)
	defer srv.Close()

	store := &fakeStore{}
	h := New(client.New(srv.URL, 1, 0), store, progress.NewHarvestProgress(), Config{})

	summary, err := h.Harvest(context.Background(), types.Scope{
		StartDate:  "20260101",
		EndDate:    "20260102",
		AgencyCNPJ: []string{"01409580000138"},
		MaxPages:   3,
	})
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if summary.Inserted.Contracts != 3 {
		t.Errorf("Inserted = %d (queria 3)", summary.Inserted.Contracts)
	}
	if store.inMemory.coverageStatus != types.StatusPartial {
		t.Errorf("status da cobertura = %q (queria %q)", store.inMemory.coverageStatus, types.StatusPartial)
	}
	if store.inMemory.coverageTotal == nil || *store.inMemory.coverageTotal != 5 {
		t.Errorf("TotalRecords da cobertura = %v (queria 5)", store.inMemory.coverageTotal)
	}
}

func TestHarvestAgencyCNPJCompletedWithoutLimit(t *testing.T) {
	srv := serverContracts(t, 5, nil)
	defer srv.Close()

	store := &fakeStore{}
	h := New(client.New(srv.URL, 1, 0), store, progress.NewHarvestProgress(), Config{})

	summary, err := h.Harvest(context.Background(), types.Scope{
		StartDate:  "20260101",
		EndDate:    "20260102",
		AgencyCNPJ: []string{"01409580000138"},
	})
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if summary.Inserted.Contracts != 5 {
		t.Errorf("Inserted = %d (queria 5)", summary.Inserted.Contracts)
	}
	if store.inMemory.coverageStatus != types.StatusCompleted {
		t.Errorf("status da cobertura = %q (queria %q)", store.inMemory.coverageStatus, types.StatusCompleted)
	}
}

func TestHarvestResumesFromLastPageOK(t *testing.T) {
	srv := serverContracts(t, 5, nil)
	defer srv.Close()

	store := &fakeStore{}
	store.inMemory.lastPageOK = 2
	h := New(client.New(srv.URL, 1, 0), store, progress.NewHarvestProgress(), Config{})

	summary, err := h.Harvest(context.Background(), types.Scope{
		StartDate:  "20260101",
		EndDate:    "20260102",
		AgencyCNPJ: []string{"01409580000138"},
	})
	if err != nil {
		t.Fatalf("harvest: %v", err)
	}
	if summary.ResumedFrom != 2 {
		t.Errorf("ResumedFrom = %d (queria 2)", summary.ResumedFrom)
	}
	// Paginas 3..5 = 3 contratos.
	if summary.Inserted.Contracts != 3 {
		t.Errorf("Inserted = %d (queria 3)", summary.Inserted.Contracts)
	}
	if store.inMemory.coverageStatus != types.StatusCompleted {
		t.Errorf("status da cobertura = %q (queria %q)", store.inMemory.coverageStatus, types.StatusCompleted)
	}
}

func TestRecordCoveragePreservesTotalOnResume(t *testing.T) {
	// Quando o harvest e retomado (lastPageOK > 0), a pagina 1 nao e consultada
	// e o totalRegistros esperado chega como nil: a cobertura deve preservar o
	// total ja registrado em vez de sobrescreve-lo com NULL.
	totalKnown := 915
	store := &fakeStore{}
	store.inMemory.coverageTotal = &totalKnown
	h := New(client.New("http://127.0.0.1:1", 1, 0), store, progress.NewHarvestProgress(), Config{})

	unit := harvestUnit{entity: types.EntityContracts, agencyCNPJ: "01409580000138"}
	err := h.recordCoverage(context.Background(), unit, types.Window{
		StartDate: "20260101",
		EndDate:   "20260102",
	}, 0, true)
	if err != nil {
		t.Fatalf("recordCoverage: %v", err)
	}

	if store.inMemory.coverageTotal == nil || *store.inMemory.coverageTotal != totalKnown {
		t.Errorf("total_registros deve ser preservado na retomada; got %v", store.inMemory.coverageTotal)
	}
}

func TestHarvestEmptyScopeRejected(t *testing.T) {
	store := &fakeStore{}
	h := New(client.New("http://127.0.0.1:1", 1, 0), store, progress.NewHarvestProgress(), Config{})

	_, err := h.Harvest(context.Background(), types.Scope{
		StartDate: "20260101",
		EndDate:   "20260102",
	})
	if err != ErrEmptyScope {
		t.Fatalf("erro = %v (queria ErrEmptyScope)", err)
	}
}

func TestHarvestInvalidAgencyCNPJRejected(t *testing.T) {
	store := &fakeStore{}
	h := New(client.New("http://127.0.0.1:1", 1, 0), store, progress.NewHarvestProgress(), Config{})

	_, err := h.Harvest(context.Background(), types.Scope{
		StartDate:  "20260101",
		EndDate:    "20260102",
		AgencyCNPJ: []string{"abc", "!@#"},
	})
	if err != ErrEmptyScope {
		t.Fatalf("erro = %v (queria ErrEmptyScope)", err)
	}
}
