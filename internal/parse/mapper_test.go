package parse

import (
	"testing"
)

func TestToContractCompleto(t *testing.T) {
	raw := Raw{
		"numeroControlePNCP":        "11470270000182-2-000321/2025",
		"numeroControlePncpCompra":  "11470270000182-1-000154/2024",
		"anoContrato":               float64(2025),
		"sequencialContrato":        float64(321),
		"objetoContrato":            "Aquisicao de materiais",
		"niFornecedor":              "07640617000110",
		"tipoPessoa":                "PJ",
		"nomeRazaoSocialFornecedor": "DISTRI. BRASIL COMER. LTDA",
		"valorInicial":              2073.58,
		"valorGlobal":               2073.58,
		"valorAcumulado":            2073.58,
		"dataAssinatura":            "2025-07-01",
		"dataVigenciaInicio":        "2025-07-01",
		"dataVigenciaFim":           "2025-12-31",
		"dataPublicacaoPncp":        "2026-01-05T16:03:49",
		"dataAtualizacaoGlobal":     "2026-01-05T16:03:49",
		"numeroContratoEmpenho":     "102203",
		"orgaoEntidade": map[string]any{
			"cnpj":        "11470270000182",
			"razaoSocial": "FUNDO MUNICIPAL DE SAUDE",
			"poderId":     "E",
			"esferaId":    "M",
		},
		"unidadeOrgao": map[string]any{
			"codigoIbge":    "5214101",
			"municipioNome": "Mutunopolis",
			"ufSigla":       "GO",
			"ufNome":        "Goias",
		},
		"tipoContrato": map[string]any{
			"id":   float64(7),
			"nome": "Empenho",
		},
	}

	contract := ToContract(raw)
	if contract == nil {
		t.Fatal("esperava linha mapeada")
	}
	if contract.ControlNumberPNCP != "11470270000182-2-000321/2025" {
		t.Errorf("ControlNumberPNCP = %q", contract.ControlNumberPNCP)
	}
	if got := *contract.PurchaseControlNumber; got != "11470270000182-1-000154/2024" {
		t.Errorf("PurchaseControlNumber = %q", got)
	}
	if got := *contract.ContractYear; got != 2025 {
		t.Errorf("ContractYear = %d", got)
	}
	if got := *contract.GlobalValue; got != 2073.58 {
		t.Errorf("GlobalValue = %f", got)
	}
	if got := *contract.AgencyCNPJ; got != "11470270000182" {
		t.Errorf("AgencyCNPJ = %q", got)
	}
	if got := *contract.IBGECode; got != "5214101" {
		t.Errorf("IBGECode = %q", got)
	}
	if got := *contract.StateAcronym; got != "GO" {
		t.Errorf("StateAcronym = %q", got)
	}
	if contract.ContractType == nil || *contract.ContractType != "Empenho" {
		t.Errorf("ContractType deveria virar a string 'Empenho' do objeto {id,nome}")
	}
	// dados_json preserva o payload cru completo, incluindo campos nao tipados.
	if len(contract.RawJSON) == 0 {
		t.Error("RawJSON vazio")
	}
	if !contains(contract.RawJSON, "numeroContratoEmpenho") {
		t.Error("RawJSON deve preservar campos nao tipados (numeroContratoEmpenho)")
	}
}

func TestToContractSemNumero(t *testing.T) {
	if contract := ToContract(Raw{"objetoContrato": "x"}); contract != nil {
		t.Fatal("sem numeroControlePNCP a linha deve ser ignorada")
	}
}

func TestToProcurementCompleto(t *testing.T) {
	raw := Raw{
		"numeroControlePNCP":       "00000000000000-0-00001/2026",
		"numeroCompra":             "20250001",
		"anoCompra":                float64(2026),
		"sequencialCompra":         float64(1),
		"modalidadeId":             float64(8),
		"modalidadeNome":           "Dispensa",
		"modoDisputaId":            float64(1),
		"modoDisputaNome":          "Eletronico",
		"situacaoCompraId":         float64(1),
		"situacaoCompraNome":       "Publicada",
		"objetoCompra":             "Aquisicao de insumos",
		"valorTotalEstimado":       1200.50,
		"valorTotalHomologado":     1180.00,
		"dataPublicacaoPncp":       "2026-01-05T16:03:49",
		"dataInclusao":             "2026-01-05T16:03:49",
		"dataAtualizacao":          "2026-01-05T16:03:49",
		"dataAberturaProposta":     "2026-02-01T09:00:00",
		"dataEncerramentoProposta": "2026-02-15T18:00:00",
		"orgaoEntidade": map[string]any{
			"cnpj":        "01409580000138",
			"razaoSocial": "ESTADO DE GOIAS",
		},
		"unidadeOrgao": map[string]any{
			"codigoIbge":    "5208707",
			"municipioNome": "Goiania",
			"ufSigla":       "GO",
		},
		"tipoInstrumentoConvocatorioCodigo": float64(1),
		"tipoInstrumentoConvocatorioNome":   "Edital",
		"srp":                               true,
		"emendaParlamentar":                 false,
		"processo":                          "2025000001",
		"linkProcessoEletronico":            "https://pncp.gov.br/processo/1",
		"linkSistemaOrigem":                 "https://compras.go.gov.br",
		"usuarioNome":                       "MARIA",
	}

	procurement := ToProcurement(raw)
	if procurement == nil {
		t.Fatal("esperava linha mapeada")
	}
	if procurement.ControlNumberPNCP != "00000000000000-0-00001/2026" {
		t.Errorf("ControlNumberPNCP = %q", procurement.ControlNumberPNCP)
	}
	if got := *procurement.PurchaseYear; got != 2026 {
		t.Errorf("PurchaseYear = %d", got)
	}
	if got := *procurement.ModalityID; got != 8 {
		t.Errorf("ModalityID = %d", got)
	}
	if got := *procurement.ModalityName; got != "Dispensa" {
		t.Errorf("ModalityName = %q", got)
	}
	if got := *procurement.EstimatedTotalValue; got != 1200.50 {
		t.Errorf("EstimatedTotalValue = %f", got)
	}
	if got := *procurement.ApprovedTotalValue; got != 1180.00 {
		t.Errorf("ApprovedTotalValue = %f", got)
	}
	if got := *procurement.AgencyCNPJ; got != "01409580000138" {
		t.Errorf("AgencyCNPJ = %q", got)
	}
	if got := *procurement.StateAcronym; got != "GO" {
		t.Errorf("StateAcronym = %q", got)
	}
	if got := *procurement.SRP; !got {
		t.Errorf("SRP = %v (queria true)", got)
	}
	if got := *procurement.BudgetAmendment; got {
		t.Errorf("BudgetAmendment = %v (queria false)", got)
	}
	if got := *procurement.BidOpeningDate; got.Year() != 2026 {
		t.Errorf("BidOpeningDate = %v", got)
	}
	if len(procurement.RawJSON) == 0 {
		t.Error("RawJSON vazio")
	}
	if !contains(procurement.RawJSON, "linkSistemaOrigem") {
		t.Error("RawJSON deve preservar o payload cru completo")
	}
}

func TestToProcurementSemNumero(t *testing.T) {
	if procurement := ToProcurement(Raw{"objetoCompra": "x"}); procurement != nil {
		t.Fatal("sem numeroControlePNCP a linha deve ser ignorada")
	}
}

func contains(b []byte, s string) bool {
	text := string(b)
	for i := 0; i+len(s) <= len(text); i++ {
		if text[i:i+len(s)] == s {
			return true
		}
	}
	return false
}
