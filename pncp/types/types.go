// Package types define os tipos compartilhados do harvest da API de Consulta
// v1 do PNCP (https://pncp.gov.br/api/consulta/v1), exportados publicamente
// para uso pelo cliente da biblioteca.
package types

import (
	"encoding/json"
	"time"
)

// -----------------------------------------------------------------------------
// Entidade harvestada pela API de Consulta v1.
// -----------------------------------------------------------------------------

const (
	// EntityContracts corresponde ao endpoint contratos (contratos assinados).
	EntityContracts = "contratos"

	// EntityProcurements corresponde ao endpoint contratacoes/publicacao
	// (contratacoes publicadas, filtradas por UF/municipio no servidor).
	EntityProcurements = "contratacoes"
)

const (
	// MinPageSize e o tamanhoPagina minimo aceito pela API (HTTP 400 abaixo).
	MinPageSize = 10

	// MaxContractsPageSize e o maximo do endpoint /contratos.
	MaxContractsPageSize = 500

	// MaxProcurementsPageSize e o maximo do endpoint /contratacoes/publicacao.
	MaxProcurementsPageSize = 50
)

// Envs de configuracao da biblioteca. A URL interna do PNCP permanece fixa na
// implementacao; as variaveis de ambiente apenas permitem sobrepor de forma
// generica (testes, proxy).
const (
	EnvPNCPBaseURL    = "DADOS_LIVRES_PNCP_BASE_URL"
	EnvMaxConcurrency = "DADOS_LIVRES_MAX_CONCORRENCIA"
	EnvDelayMS        = "DADOS_LIVRES_DELAY_MS"
	EnvMaxPages       = "DADOS_LIVRES_HARVEST_MAX_PAGINAS"
)

// Storage (backend de saida) e arquivo de saida da planilha.
const (
	EnvStorage      = "DADOS_LIVRES_PERSISTENCIA"
	EnvXLSXPath     = "DADOS_LIVRES_XLSX_CAMINHO"
	StoragePostgres = "postgres"
	StorageXLSX     = "xlsx"

	// XLSXDefaultPath e o arquivo de saida padrao do backend xlsx
	// (subdiretorios criados automaticamente na primeira gravacao).
	XLSXDefaultPath = "internal/database/xlsx/dados-livres.xlsx"
)

// Status da cobertura de um escopo na dados_livres_pncp_meta.
const (
	StatusCompleted = "concluido"
	StatusError     = "error"
	StatusPartial   = "parcial"
)

// -----------------------------------------------------------------------------
// Escopo e resumo do harvest.
// -----------------------------------------------------------------------------

// Scope define o recorte do harvest. Sao suportadas tres dimensoes de recorte,
// que definem o endpoint consultado:
//   - States e/ou Municipalities: contratacoes publicadas (/contratacoes/publicacao).
//   - AgencyCNPJ: contratos do orgao (/contratos, filtro server-side).
//
// Todas as demais operacoes (datas, paginacao) sao configuracao da varredura.
type Scope struct {
	// StartDate e EndDate no formato AAAAMMDD. Sobrepoem Month/Years.
	StartDate string `json:"data_inicial"`
	EndDate   string `json:"data_final"`

	// Month no formato YYYY-MM: janela do mes inteiro. Sobrepoe Years.
	Month string `json:"mes"`

	// Years: de 1o de janeiro do menor ano ate o fim do maior (ou hoje).
	Years []int `json:"anos"`

	// States (siglas de 2 letras): recorte de contratacoes por UF.
	States []string `json:"ufs"`

	// Municipalities (codigo IBGE de 7 digitos): recorte de contratacoes por
	// UF e municipio quando combinado com States.
	Municipalities []string `json:"municipios"`

	// AgencyCNPJ (somente digitos): recorte de contratos do orgao, enviado como
	// cnpjOrgao ao endpoint /contratos (filtro aplicado no servidor). Aceita
	// uma lista; cada CNPJ vira uma varredura propria.
	AgencyCNPJ []string `json:"orgaos_cnpj"`

	// MaxPages limita o numero de paginas por varredura (entidade, UF,
	// municipio, modalidade, orgao). 0 = sem limite (anti-runaway configurado
	// via env quando necessario).
	MaxPages int `json:"max_paginas"`

	// PageSize (clamp por endpoint). 0 = maximo do endpoint.
	PageSize int `json:"tamanho_pagina"`
}

// Window descreve o periodo efetivamente harvestado (AAAAMMDD).
type Window struct {
	StartDate string `json:"data_inicial"`
	EndDate   string `json:"data_final"`
}

// Inserted contabiliza registros mapeados e persistidos por entidade.
type Inserted struct {
	Contracts    int `json:"contratos"`
	Procurements int `json:"contratacoes"`
}

// HarvestSummary e o resultado do build/harvest.
type HarvestSummary struct {
	Window       Window    `json:"janela"`
	States       []string  `json:"ufs"`
	Inserted     Inserted  `json:"inseridos"`
	Skipped      int       `json:"pulados"`
	ResumedFrom  int       `json:"retomado_de,omitempty"`
	TotalRecords int       `json:"total_registros"`
	UpdatedAt    time.Time `json:"atualizado_em"`
}

// Coverage descreve um escopo harvestado (linha da dados_livres_pncp_meta).
type Coverage struct {
	Entity       string     `json:"entidade"`
	State        string     `json:"uf"`
	Municipality string     `json:"municipio,omitempty"`
	AgencyCNPJ   string     `json:"orgao_cnpj,omitempty"`
	Modality     int        `json:"modalidade"`
	StartDate    string     `json:"data_inicial"`
	EndDate      string     `json:"data_final"`
	Status       string     `json:"status"`
	TotalRecords *int       `json:"total_registros,omitempty"`
	LastPageOK   int        `json:"ultima_pagina_ok"`
	ErrorPage    *int       `json:"pagina_erro,omitempty"`
	UpdatedAt    *time.Time `json:"atualizado_em,omitempty"`
}

// IndexStatus do indice (para consulta de estado da biblioteca).
type IndexStatus struct {
	Exists    bool       `json:"existe"`
	UpdatedAt *time.Time `json:"atualizado_em"`
	Records   int        `json:"registros"`
	Window    Window     `json:"janela"`
	Coverages []Coverage `json:"coberturas"`
}

// ProgressEvent descreve o andamento do harvest em um instante.
type ProgressEvent struct {
	CurrentEntity   string  `json:"entidade_atual"`
	CurrentState    string  `json:"uf_atual,omitempty"`
	CurrentPage     int     `json:"pagina_atual"`
	Contracts       int     `json:"contratos"`
	Procurements    int     `json:"contratacoes"`
	Pages           int     `json:"paginas"`
	Errors          int     `json:"erros"`
	DurationSeconds float64 `json:"duracao_segundos"`
	Timestamp       string  `json:"timestamp"`
}

// -----------------------------------------------------------------------------
// Linha mapeada (espelho do mapper de /contratos).
// -----------------------------------------------------------------------------

// Contract corresponde a um registro de /contratos (contrato assinado).
// UF/municipio vem de unidadeOrgao. O payload cru completo e preservado em RawJSON.
type Contract struct {
	ControlNumberPNCP     string
	PurchaseControlNumber *string
	ContractYear          *int
	ContractSequential    *int
	ContractObject        string
	SupplierID            *string
	PersonType            *string
	SupplierName          string
	SubcontractorID       *string
	SubcontractorName     *string
	InitialValue          *float64
	GlobalValue           *float64
	AccumulatedValue      *float64
	SignatureDate         *time.Time
	StartDate             *time.Time
	EndDate               *time.Time
	PublicationDate       *time.Time
	UpdateDate            *time.Time
	AgencyCNPJ            *string
	AgencyName            *string
	IBGECode              *string
	MunicipalityName      *string
	StateAcronym          *string
	ContractType          *string
	RawJSON               json.RawMessage
}

// Procurement corresponde a um registro de /contratacoes/publicacao
// (contratacao publicada), corrigido por UF/municipio no servidor.
type Procurement struct {
	ControlNumberPNCP     string
	PurchaseNumber        *string
	PurchaseYear          *int
	PurchaseSequential    *int
	ModalityID            *int
	ModalityName          *string
	DisputeModeID         *int
	DisputeModeName       *string
	PurchaseStatusID      *int
	PurchaseStatusName    *string
	PurchaseObject        string
	EstimatedTotalValue   *float64
	ApprovedTotalValue    *float64
	PublicationDate       *time.Time
	InclusionDate         *time.Time
	UpdateDate            *time.Time
	GlobalUpdateDate      *time.Time
	BidOpeningDate        *time.Time
	BidClosingDate        *time.Time
	AgencyCNPJ            *string
	AgencyName            *string
	IBGECode              *string
	MunicipalityName      *string
	StateAcronym          *string
	UnitName              *string
	InstrumentTypeCode    *int
	InstrumentTypeName    *string
	SRP                   *bool
	BudgetAmendment       *bool
	Process               string
	ElectronicProcessLink string
	SourceSystemLink      string
	UserName              string
	RawJSON               json.RawMessage
}

// -----------------------------------------------------------------------------
// Resultados de busca.
// -----------------------------------------------------------------------------

// Supplier agrupa contratos por fornecedor (nome).
type Supplier struct {
	SupplierID string  `json:"ni_fornecedor"`
	Name       string  `json:"nome"`
	Contracts  int64   `json:"contratos"`
	TotalValue float64 `json:"valor_total"`
}

// SupplierContract e um contrato listado por ContractsBySupplier.
type SupplierContract struct {
	ControlNumberPNCP string     `json:"numero_controle_pncp"`
	ContractObject    string     `json:"objeto_contrato"`
	SupplierName      string     `json:"nome_razao_social_fornecedor"`
	SupplierID        *string    `json:"ni_fornecedor,omitempty"`
	GlobalValue       *float64   `json:"valor_global,omitempty"`
	InitialValue      *float64   `json:"valor_inicial,omitempty"`
	SignatureDate     *time.Time `json:"data_assinatura,omitempty"`
	StartDate         *time.Time `json:"data_vigencia_inicio,omitempty"`
	EndDate           *time.Time `json:"data_vigencia_fim,omitempty"`
	AgencyCNPJ        *string    `json:"orgao_cnpj,omitempty"`
	AgencyName        *string    `json:"orgao_razao_social,omitempty"`
	IBGECode          *string    `json:"codigo_ibge,omitempty"`
	StateAcronym      *string    `json:"uf_sigla,omitempty"`
}

// SupplierAggregation agrega o gasto por orgao/municipio/uf.
type SupplierAggregation struct {
	Key        string  `json:"chave"`
	Label      string  `json:"rotulo"`
	Contracts  int64   `json:"contratos"`
	TotalValue float64 `json:"valor_total"`
}

// SupplierContractsMeta resume o resultado de ContractsBySupplier.
type SupplierContractsMeta struct {
	SupplierID     string  `json:"ni"`
	Limit          int     `json:"limite"`
	GroupBy        string  `json:"agrupar_por"`
	TotalContracts int64   `json:"total_contratos"`
	TotalValue     float64 `json:"valor_total"`
}

// SupplierContractsResult e o retorno de ContractsBySupplier.
type SupplierContractsResult struct {
	Meta        SupplierContractsMeta `json:"meta"`
	Aggregation []SupplierAggregation `json:"agregacao"`
	Results     []SupplierContract    `json:"resultados"`
}

// ContractSearch e um contrato assinado retornado na busca por escopo.
// UF/municipio vem dos campos enriquecidos do proprio contrato.
type ContractSearch struct {
	ControlNumberPNCP     string     `json:"numero_controle_pncp"`
	PurchaseControlNumber *string    `json:"numero_controle_pncp_compra,omitempty"`
	ContractYear          *int       `json:"ano_contrato,omitempty"`
	ContractSequential    *int       `json:"sequencial_contrato,omitempty"`
	ContractObject        string     `json:"objeto_contrato"`
	SupplierID            *string    `json:"ni_fornecedor,omitempty"`
	SupplierName          string     `json:"nome_razao_social_fornecedor"`
	InitialValue          *float64   `json:"valor_inicial,omitempty"`
	GlobalValue           *float64   `json:"valor_global,omitempty"`
	AccumulatedValue      *float64   `json:"valor_acumulado,omitempty"`
	SignatureDate         *time.Time `json:"data_assinatura,omitempty"`
	StartDate             *time.Time `json:"data_vigencia_inicio,omitempty"`
	EndDate               *time.Time `json:"data_vigencia_fim,omitempty"`
	PublicationDate       *time.Time `json:"data_publicacao_pncp,omitempty"`
	AgencyCNPJ            *string    `json:"orgao_cnpj,omitempty"`
	AgencyName            *string    `json:"orgao_razao_social,omitempty"`
	IBGECode              *string    `json:"codigo_ibge,omitempty"`
	MunicipalityName      *string    `json:"municipio_nome,omitempty"`
	StateAcronym          *string    `json:"uf_sigla,omitempty"`
}

// ProcurementSearch e uma contratacao publicada retornada na busca.
type ProcurementSearch struct {
	ControlNumberPNCP   string     `json:"numero_controle_pncp"`
	PurchaseNumber      *string    `json:"numero_compra,omitempty"`
	PurchaseYear        *int       `json:"ano_compra,omitempty"`
	PurchaseSequential  *int       `json:"sequencial_compra,omitempty"`
	ModalityID          *int       `json:"modalidade_id,omitempty"`
	ModalityName        *string    `json:"modalidade_nome,omitempty"`
	PurchaseObject      string     `json:"objeto_compra"`
	EstimatedTotalValue *float64   `json:"valor_total_estimado,omitempty"`
	ApprovedTotalValue  *float64   `json:"valor_total_homologado,omitempty"`
	PublicationDate     *time.Time `json:"data_publicacao_pncp,omitempty"`
	AgencyCNPJ          *string    `json:"orgao_cnpj,omitempty"`
	AgencyName          *string    `json:"orgao_razao_social,omitempty"`
	IBGECode            *string    `json:"codigo_ibge,omitempty"`
	MunicipalityName    *string    `json:"municipio_nome,omitempty"`
	StateAcronym        *string    `json:"uf_sigla,omitempty"`
}

// SearchFilter descreve o recorte da busca por UF/municipio/modalidade/orgao
// (contratos e contratacoes indexadas).
type SearchFilter struct {
	State                string
	Municipality         string
	AgencyCNPJ           string
	Modality             int
	StartPublicationDate string
	EndPublicationDate   string
}
