package xlsx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/danyele/dados-livres/pncp/types"
)

func TestUpsertAndStatusRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contratos.xlsx")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("planilha nao deveria existir antes da primeira gravacao")
	}

	ctx := context.Background()
	now := time.Now().UTC()

	if err := s.UpsertContracts(ctx, []types.Contract{
		{
			ControlNumberPNCP:     "11470270000182-2-000321/2025",
			PurchaseControlNumber: strPointer("11470270000182-1-000154/2024"),
			ContractYear:          intPointer(2025),
			ContractObject:        "Aquisicao",
			SupplierID:            strPointer("07640617000110"),
			SupplierName:          "DISTRI. BRASIL COMER. LTDA",
			GlobalValue:           floatPointer(2073.58),
			PublicationDate:       &now,
			AgencyCNPJ:            strPointer("11470270000182"),
			IBGECode:              strPointer("5214101"),
			StateAcronym:          strPointer("GO"),
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("planilha deveria ter sido criada: %v", err)
	}

	total, err := s.CountRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 {
		t.Fatalf("total = %d, esperava 1", total)
	}

	// upsert idempotente pela chave
	if err := s.UpsertContracts(ctx, []types.Contract{
		{
			ControlNumberPNCP: "11470270000182-2-000321/2025",
			SupplierName:      "NOME ATUALIZADO LTDA",
		},
		{
			ControlNumberPNCP: "outro-0001",
			SupplierName:      "Segundo",
		},
	}); err != nil {
		t.Fatal(err)
	}
	total, err = s.CountRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("total apos upsert = %d, esperava 2", total)
	}

	// reabrir o arquivo e verificar estado consistente
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	total, err = s2.CountRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 {
		t.Fatalf("reaberto total = %d, esperava 2", total)
	}
}

func TestCoverages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "meta.xlsx")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	s.RecordPageProgress(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0, 3, intPointer(915))
	s.RecordPageProgress(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0, 19, intPointer(915))
	if err := s.RecordCoverage(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0, types.StatusCompleted, intPointer(915)); err != nil {
		t.Fatal(err)
	}

	last, err := s.LastPageOK(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0)
	if err != nil {
		t.Fatal(err)
	}
	if last != 19 {
		t.Fatalf("last page = %d, esperava 19", last)
	}

	completed, err := s.CoverageCompleted(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("cobertura deveria existir como concluida")
	}

	listed, err := s.ListCoverages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 {
		t.Fatalf("coberturas = %d", len(listed))
	}
	if listed[0].Status != types.StatusCompleted || listed[0].LastPageOK != 19 {
		t.Fatalf("cobertura inesperada: %+v", listed[0])
	}
	if listed[0].TotalRecords == nil || *listed[0].TotalRecords != 915 {
		t.Fatalf("TotalRecords inesperado: %+v", listed[0].TotalRecords)
	}

	// falha zera para status error e guarda a pagina
	if err := s.RecordPageFailure(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0, 5); err != nil {
		t.Fatal(err)
	}
	completed, _ = s.CoverageCompleted(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0)
	if completed {
		t.Fatal("apos falha a cobertura nao deveria constar concluida")
	}
	status := mustStatus(t, s, ctx)
	if status.Exists != true {
		t.Fatal("status deveria existir")
	}

	// reabrir e conferir persistencia das coberturas
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	last, _ = s2.LastPageOK(ctx, "contratos", "GO", "5215231", "", "20260101", "20260102", 0)
	if last != 19 {
		t.Fatalf("reaberto last page = %d, esperava 19", last)
	}
}

func TestUpsertProcurementsAndStatusRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "contratacoes.xlsx")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	if err := s.UpsertProcurements(ctx, []types.Procurement{
		{
			ControlNumberPNCP:   "00000000000000-0-00001/2026",
			PurchaseNumber:      strPointer("20250001"),
			PurchaseYear:        intPointer(2026),
			ModalityID:          intPointer(8),
			ModalityName:        strPointer("Dispensa"),
			PurchaseObject:      "Aquisicao de materiais",
			EstimatedTotalValue: floatPointer(1200.50),
			PublicationDate:     &now,
			AgencyName:          strPointer("ESTADO DE GOIAS"),
			IBGECode:            strPointer("5208707"),
			MunicipalityName:    strPointer("Goiania"),
			StateAcronym:        strPointer("GO"),
			SRP:                 boolPointer(false),
			BudgetAmendment:     boolPointer(true),
		},
	}); err != nil {
		t.Fatal(err)
	}

	status := mustStatus(t, s, ctx)
	if !status.Exists {
		t.Fatal("status deveria existir com contratacoes")
	}
	if status.Records != 1 {
		t.Fatalf("Records do status = %d (esperava 1, soma contratos + contratacoes)", status.Records)
	}

	// upsert idempotente pela chave
	if err := s.UpsertProcurements(ctx, []types.Procurement{
		{
			ControlNumberPNCP: "00000000000000-0-00001/2026",
			ModalityID:        intPointer(8),
		},
	}); err != nil {
		t.Fatal(err)
	}
	status = mustStatus(t, s, ctx)
	if status.Records != 1 {
		t.Fatalf("Records apos upsert = %d (esperava 1)", status.Records)
	}

	// reabrir o arquivo e verificar estado consistente
	s2, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	status = mustStatus(t, s2, ctx)
	if status.Records != 1 {
		t.Fatalf("reaberto Records = %d (esperava 1)", status.Records)
	}
}

func TestCoverageWithAgencyCNPJ(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-orgao.xlsx")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	// Dois orgaos distintos na mesma UF/periodo nao devem colidir na cobertura.
	if err := s.RecordCoverage(ctx, "contratos", "GO", "", "01409580000138", "20260101", "20260102", 0, types.StatusCompleted, intPointer(915)); err != nil {
		t.Fatal(err)
	}
	if err := s.RecordCoverage(ctx, "contratos", "GO", "", "82928664000180", "20260101", "20260102", 0, types.StatusCompleted, intPointer(3)); err != nil {
		t.Fatal(err)
	}

	listed, err := s.ListCoverages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 2 {
		t.Fatalf("coberturas = %d (esperava 2)", len(listed))
	}
	for _, c := range listed {
		if c.AgencyCNPJ == "" {
			t.Fatalf("cobertura sem orgao: %+v", c)
		}
	}

	completed, err := s.CoverageCompleted(ctx, "contratos", "GO", "", "01409580000138", "20260101", "20260102", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !completed {
		t.Fatal("cobertura do orgao deveria existir como concluida")
	}
}

func TestSaveCreatesPathDirectory(t *testing.T) {
	// O usuario pode apontar DADOS_LIVRES_XLSX_CAMINHO para um caminho com
	// subdiretorios ainda inexistentes; o repositorio deve cria-los ao salvar.
	path := filepath.Join(t.TempDir(), "nao", "existe", "ainda", "contratos.xlsx")
	s, err := New(path)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	if err := s.UpsertContracts(ctx, []types.Contract{
		{
			ControlNumberPNCP: "11470270000182-2-000321/2025",
			SupplierName:      "DISTRI. BRASIL COMER. LTDA",
		},
	}); err != nil {
		t.Fatalf("salvar com diretorio inexistente: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("planilha deveria existir em %s: %v", path, err)
	}
}

func mustStatus(t *testing.T, s *Store, ctx context.Context) *types.IndexStatus {
	t.Helper()
	status, err := s.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return status
}

func strPointer(s string) *string {
	return &s
}

func intPointer(i int) *int {
	return &i
}

func floatPointer(f float64) *float64 {
	return &f
}

func boolPointer(b bool) *bool {
	return &b
}
