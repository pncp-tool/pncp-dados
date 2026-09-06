// Package parse contem os mapeadores PUROS (sem rede, sem IO) dos registros
// crus da API de Consulta v1 do PNCP para as linhas persistidas.
//
// Shape verificado:
//   - /contratos usa objetos ANINHADOS orgaoEntidade{cnpj,razaoSocial,...} e
//     unidadeOrgao{codigoIbge,municipioNome,ufSigla,...}.
//   - /contratacoes/publicacao expoe a modalidade, o objeto e os valores da
//     compra nos proprios campos do registro.
package parse

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/danyele/dados-livres/pncp/types"
)

// Raw e um registro cru decodificado da API.
type Raw = map[string]any

func asObject(value any) Raw {
	if m, ok := value.(map[string]any); ok {
		return m
	}
	return Raw{}
}

func str(value any) *string {
	switch v := value.(type) {
	case string:
		return &v
	case nil:
		return nil
	default:
		s := toString(value)
		return &s
	}
}

func strOrEmpty(value any) string {
	if s := str(value); s != nil {
		return *s
	}
	return ""
}

func toString(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	}
	return ""
}

func numFloat(value any) *float64 {
	switch v := value.(type) {
	case float64:
		return &v
	case string:
		if v = strings.TrimSpace(v); v != "" {
			n, err := strconv.ParseFloat(v, 64)
			if err == nil {
				return &n
			}
		}
	}
	return nil
}

func numInt(value any) *int {
	switch v := value.(type) {
	case float64:
		n := int(v)
		return &n
	case string:
		if v = strings.TrimSpace(v); v != "" {
			n, err := strconv.Atoi(v)
			if err == nil {
				return &n
			}
		}
	}
	return nil
}

func boolPtr(value any) *bool {
	if v, ok := value.(bool); ok {
		return &v
	}
	return nil
}

// PickControlNumber retorna o primeiro valor nao vazio entre as chaves
// informadas (a API usa numeroControlePNCP).
func PickControlNumber(raw Raw, keys ...string) string {
	for _, key := range keys {
		if s := strOrEmpty(raw[key]); s != "" {
			return s
		}
	}
	return ""
}

// ToContract mapeia um registro cru de /contratos (contrato assinado).
func ToContract(raw Raw) *types.Contract {
	number := PickControlNumber(raw, "numeroControlePNCP")
	if number == "" {
		return nil
	}

	orgao := asObject(raw["orgaoEntidade"])
	unidade := asObject(raw["unidadeOrgao"])
	tipoContrato := raw["tipoContrato"]
	if obj, ok := tipoContrato.(map[string]any); ok {
		tipoContrato = obj["nome"]
	}

	return &types.Contract{
		ControlNumberPNCP:     number,
		PurchaseControlNumber: str(raw["numeroControlePncpCompra"]),
		ContractYear:          numInt(raw["anoContrato"]),
		ContractSequential:    numInt(raw["sequencialContrato"]),
		ContractObject:        strOrEmpty(raw["objetoContrato"]),
		SupplierID:            str(raw["niFornecedor"]),
		PersonType:            str(raw["tipoPessoa"]),
		SupplierName:          strOrEmpty(raw["nomeRazaoSocialFornecedor"]),
		SubcontractorID:       str(raw["niFornecedorSubContratado"]),
		SubcontractorName:     str(raw["nomeFornecedorSubContratado"]),
		InitialValue:          numFloat(raw["valorInicial"]),
		GlobalValue:           numFloat(raw["valorGlobal"]),
		AccumulatedValue:      numFloat(raw["valorAcumulado"]),
		SignatureDate:         parseTime(raw["dataAssinatura"]),
		StartDate:             parseTime(raw["dataVigenciaInicio"]),
		EndDate:               parseTime(raw["dataVigenciaFim"]),
		PublicationDate:       parseTime(raw["dataPublicacaoPncp"]),
		UpdateDate:            firstTime(raw["dataAtualizacaoGlobal"], raw["dataAtualizacao"]),
		AgencyCNPJ:            str(orgao["cnpj"]),
		AgencyName:            str(orgao["razaoSocial"]),
		IBGECode:              str(unidade["codigoIbge"]),
		MunicipalityName:      str(unidade["municipioNome"]),
		StateAcronym:          str(unidade["ufSigla"]),
		ContractType:          str(tipoContrato),
		RawJSON:               rawJSON(raw),
	}
}

// ToProcurement mapeia um registro cru de /contratacoes/publicacao
// (contratacao publicada).
func ToProcurement(raw Raw) *types.Procurement {
	number := PickControlNumber(raw, "numeroControlePNCP")
	if number == "" {
		return nil
	}

	orgao := asObject(raw["orgaoEntidade"])
	unidade := asObject(raw["unidadeOrgao"])

	return &types.Procurement{
		ControlNumberPNCP:     number,
		PurchaseNumber:        str(raw["numeroCompra"]),
		PurchaseYear:          numInt(raw["anoCompra"]),
		PurchaseSequential:    numInt(raw["sequencialCompra"]),
		ModalityID:            numInt(raw["modalidadeId"]),
		ModalityName:          str(raw["modalidadeNome"]),
		DisputeModeID:         numInt(raw["modoDisputaId"]),
		DisputeModeName:       str(raw["modoDisputaNome"]),
		PurchaseStatusID:      numInt(raw["situacaoCompraId"]),
		PurchaseStatusName:    str(raw["situacaoCompraNome"]),
		PurchaseObject:        strOrEmpty(raw["objetoCompra"]),
		EstimatedTotalValue:   numFloat(raw["valorTotalEstimado"]),
		ApprovedTotalValue:    numFloat(raw["valorTotalHomologado"]),
		PublicationDate:       parseTime(raw["dataPublicacaoPncp"]),
		InclusionDate:         parseTime(raw["dataInclusao"]),
		UpdateDate:            parseTime(raw["dataAtualizacao"]),
		GlobalUpdateDate:      firstTime(raw["dataAtualizacaoGlobal"], raw["dataAtualizacao"]),
		BidOpeningDate:        parseTime(raw["dataAberturaProposta"]),
		BidClosingDate:        parseTime(raw["dataEncerramentoProposta"]),
		AgencyCNPJ:            str(orgao["cnpj"]),
		AgencyName:            str(orgao["razaoSocial"]),
		IBGECode:              str(unidade["codigoIbge"]),
		MunicipalityName:      str(unidade["municipioNome"]),
		StateAcronym:          str(unidade["ufSigla"]),
		UnitName:              str(unidade["nomeUnidade"]),
		InstrumentTypeCode:    numInt(raw["tipoInstrumentoConvocatorioCodigo"]),
		InstrumentTypeName:    str(raw["tipoInstrumentoConvocatorioNome"]),
		SRP:                   boolPtr(raw["srp"]),
		BudgetAmendment:       boolPtr(raw["emendaParlamentar"]),
		Process:               strOrEmpty(raw["processo"]),
		ElectronicProcessLink: strOrEmpty(raw["linkProcessoEletronico"]),
		SourceSystemLink:      strOrEmpty(raw["linkSistemaOrigem"]),
		UserName:              strOrEmpty(raw["usuarioNome"]),
		RawJSON:               rawJSON(raw),
	}
}

// rawJSON serializa o registro cru completo para persistir em dados_json.
// Falha de marshaling resulta em nil (nao derruba o registro).
func rawJSON(raw Raw) json.RawMessage {
	b, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	return json.RawMessage(b)
}

func firstTime(values ...any) *time.Time {
	for _, v := range values {
		if t := parseTime(v); t != nil {
			return t
		}
	}
	return nil
}

var timeLayouts = []string{
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02T15:04:05.999999999",
	"2006-01-02T15:04:05",
	"2006-01-02",
}

// parseTime converte tolerantemente strings ISO/datas da API para time.Time
// (UTC quando nao ha fuso informado). Falha de parse resulta em nil.
func parseTime(value any) *time.Time {
	s := strOrEmpty(value)
	if s == "" {
		return nil
	}
	s = strings.TrimSpace(s)
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			utc := t.UTC()
			return &utc
		}
	}
	return nil
}
