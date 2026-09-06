# dados-livres

Biblioteca Go que constrói um índice offline de contratos e contratações do
PNCP, pagina a API de Consulta v1 (`https://pncp.gov.br/api/consulta/v1`)
e persiste os dados em Postgres (padrão) ou em planilha XLSX.


| Modo | Recorte | Endpoint da API |
|------|---------|-----------------|
| UF | `--uf GO` | `/contratacoes/publicacao?uf=GO` |
| UF + município | `--uf GO --municipio 5208707` | `/contratacoes/publicacao?uf=GO&codigoMunicipioIbge=5208707` |
| Órgão | `--cnpj-orgao 01409580000138` | `/contratos?cnpjOrgao=01409580000138` |

Um escopo precisa de pelo menos uma dimensão. Escopo vazio retorna
`harvest.ErrEmptyScope`.

## Requisitos

- Go 1.22+
- Postgres 15+ (apenas para persistência com  `postgres`)


### Configuração

`dadoslivres.Options` configura a biblioteca.

```go
cliente, err := dadoslivres.New(ctx, dadoslivres.Options{
	Postgres: &dadoslivres.PostgresConfig{
		Host:     "localhost",
		Port:     "5432",
		User:     "postgres",
		Password: "sua_senha",
		Database: "tse_data",
	},
})
defer cliente.Close()
```

| Campo | Default | Descrição |
|-------|---------|-----------|
| `Storage string` | `postgres` | Backend de saída: `postgres` ou `xlsx` |
| `CustomStorage storage.Storage` | `nil` | Backend próprio. Ignora `Storage`. |
| `Postgres *PostgresConfig` | `nil` | Conexão do backend `postgres`. Obrigatório nesse modo. |
| `XLSXPath string` | `internal/database/xlsx/dados-livres.xlsx` | Arquivo XLSX de saída |
| `BaseURL string` | URL interna do PNCP | Override da URL da API (testes, proxy) |
| `MaxConcurrency int` | `1` | Requisições simultâneas |
| `DelayMS int` | `0` | Intervalo mínimo entre requisições |
| `MaxPages int` | `0` | Limite de páginas por escopo |
| `ApplyMigrations *bool` | `true` | Aplica as migrations ao abrir (Postgres) |

`PostgresConfig` espelha a conexão:

| Campo | Default |
|-------|---------|
| `Host` | `127.0.0.1` |
| `Port` | `5432` |
| `User` | obrigatório |
| `Password` | obrigatório |
| `Database` | obrigatório |
| `MaxConns` | `10` |
| `MinConns` | `2` |
| `MaxConnLifetime` | `30m` |
| `MaxConnIdleTime` | `5m` |

Host, porta e pool têm default quando zerados. `User`, `Password` e `Database`
devem ser informados, senha não tem default.

### Métodos do Cliente

| Método | Descrição |
|--------|-----------|
| `New(ctx, Options)` | Cria o cliente e abre o backend |
| `Harvest(ctx, Scope)` | Página o escopo e persiste. Retorna `*types.HarvestSummary`. |
| `Progress()` | Andamento atual. Retorna `types.ProgressEvent`. |
| `Status(ctx)` | Estado agregado do índice. Retorna `*types.IndexStatus`. |
| `ListCoverages(ctx)` | Escopos registrados. Retorna `[]types.Coverage`. |
| `CountRecords(ctx)` | Nº de contratos persistidos |
| `SuppliersByName(ctx, nome, limite)` | Busca fornecedores por razão social (Postgres) |
| `ContractsBySupplier(ctx, ni, agruparPor, limite)` | Contratos de um fornecedor (Postgres) |
| `SearchContracts(ctx, SearchFilter)` | Contratos publicados no recorte (Postgres) |
| `SearchProcurements(ctx, SearchFilter)` | Contratações publicadas no recorte (Postgres) |
| `Close()` | Libera o pool Postgres ou o arquivo XLSX |

### types.Scope

| Campo | Formato | Observação |
|-------|---------|------------|
| `StartDate` / `EndDate` | `AAAAMMDD` | Sobrepõem `Month`/`Years` |
| `Month` | `YYYY-MM` | Janela do mês inteiro. Sobrepõe `Years`. |
| `Years` | `[]int` | De 1º de janeiro do menor ano até o fim do maior, ou até hoje |
| `States` | `[]string` | Siglas de 2 letras. Harvesta contratações por UF. |
| `Municipalities` | `[]string` | Código IBGE de 7 dígitos. Combina com `States`. |
| `AgencyCNPJ` | `[]string` | CNPJs de órgãos (só dígitos). Harvesta contratos. |
| `MaxPages` | `int` | `0` = sem limite |
| `PageSize` | `int` | Clamp ao máximo do endpoint (contratos `500`, contratações `50`) |

### Exemplo mínimo (Postgres)

```go
package main

import (
	"context"
	"fmt"

	"github.com/danyele/dados-livres"
	"github.com/danyele/dados-livres/pncp/types"
)

func main() {
	ctx := context.Background()

	cliente, err := dadoslivres.New(ctx, dadoslivres.Options{
		Storage: "postgres",
		Postgres: &dadoslivres.PostgresConfig{
			Host:     "localhost",
			Port:     "5432",
			User:     "postgres",
			Password: "sua_senha",
			Database: "tse_data",
		},
	})
	if err != nil {
		panic(err)
	}
	defer cliente.Close()

	resumo, err := cliente.Harvest(ctx, types.Scope{
		StartDate:      "20260101",
		EndDate:        "20260105",
		States:         []string{"GO"},
		Municipalities: []string{"5208707"},
	})
	if err != nil {
		panic(err)
	}
	fmt.Printf("contratacoes=%d pulados=%d retomado_de=%d\n",
		resumo.Inserted.Procurements, resumo.Skipped, resumo.ResumedFrom)
}
```

Para contratos de um órgão, use `AgencyCNPJ` no lugar de `States`. O resultado
vem em `resumo.Inserted.Contracts`.

### Backend próprio

Implemente `storage.Storage` (contratos, contratações, cobertura e estado) e
opcionalmente `storage.Searcher` (buscas indexadas). Injete com
`Options.CustomStorage`.

```go
cliente, _ := dadoslivres.New(ctx, dadoslivres.Options{
	CustomStorage: meuBackend,
})
```

## Saída XLSX

Sem configuração, a planilha é salva em `internal/database/xlsx/dados-livres.xlsx`.
Os subdiretórios são criados automaticamente. O caminho é configurável em dois
níveis:

1. Em código: `dadoslivres.Options{XLSXPath: "/tmp/contratacoes-go.xlsx"}`
2. Na CLI: `harvest --caminho-xlsx /tmp/contratacoes-go.xlsx`, ou env `DADOS_LIVRES_XLSX_CAMINHO`

```go
cliente, _ := dadoslivres.New(ctx, dadoslivres.Options{
	Storage:  "xlsx",
	XLSXPath: "relatorios/contratacoes-go.xlsx",
})
```

O backend XLSX escreve as planilhas `contratos`, `contratacoes` e `meta`. O
upsert é idempotente pela chave `numero_controle_pncp`, então re-executar não
duplica. A planilha fica em memória e é reescrita a cada gravação via arquivo
temporário `.xlsx.tmp`, depois renomeado para `.xlsx`. Um arquivo existente é
relido no início, e a cobertura continua de onde parou.

## Configuração por ambiente (CLI)

Os comandos de linha de comando leem variáveis `DB_*` e `DADOS_LIVRES_*`. A
biblioteca em código não lê ambiente.

### Conexão Postgres

| Variável | Default | Descrição |
|----------|---------|-----------|
| `DB_HOST` | `localhost` | Host |
| `DB_PORT` | `5432` | Porta |
| `DB_USER` | `postgres` | Usuário |
| `DB_PASSWORD` | obrigatória | Senha. Sem default. |
| `DB_NAME` | `tse_data` | Banco de dados |
| `DADOS_LIVRES_DB_HOST` | `DB_HOST` | Sobrepõe `DB_HOST` |
| `DADOS_LIVRES_DB_PORT` | `DB_PORT` | Sobrepõe `DB_PORT` |
| `DADOS_LIVRES_DB_USER` | `DB_USER` | Sobrepõe `DB_USER` |
| `DADOS_LIVRES_DB_PASSWORD` | `DB_PASSWORD` | Sobrepõe `DB_PASSWORD` |
| `DADOS_LIVRES_DB_NAME` | `DB_NAME` | Sobrepõe `DB_NAME` |
| `DADOS_LIVRES_DB_MAX_CONNS` | `10` | Máximo de conexões no pool |

### Harvest

| Variável | Default | Descrição |
|----------|---------|-----------|
| `DADOS_LIVRES_PERSISTENCIA` | `postgres` | Backend de saída |
| `DADOS_LIVRES_XLSX_CAMINHO` | `internal/database/xlsx/dados-livres.xlsx` | Arquivo XLSX de saída |
| `DADOS_LIVRES_PNCP_BASE_URL` | `https://pncp.gov.br/api/consulta/v1` | URL interna da API |
| `DADOS_LIVRES_MAX_CONCORRENCIA` | `1` | Requisições simultâneas |
| `DADOS_LIVRES_DELAY_MS` | `0` | Intervalo mínimo entre requisições |
| `DADOS_LIVRES_HARVEST_MAX_PAGINAS` | `0` | Limite de páginas por escopo |


### Aplicar as migrations

Duas formas.

Ao chamar `dadoslivres.New` com backend `postgres`, as migrations
embutidas são aplicadas. Desligue com `ApplyMigrations` apontando para `false`.

As migrations são idempotentes. Rodar de novo responde
`nenhuma migracao pendente`. A versão aplicada fica em `dados_livres_migrations`.
As definições ficam em `internal/database/migrations/`. As migrations aplicam
somente as tabelas de prefixo `dados_livres_*`. O schema `pncp_*` do oicp não é
tocado.

## Comportamento da API

- `/contratacoes/publicacao` aceita `uf` e `codigoMunicipioIbge`.
- `/contratos` aceita `cnpjOrgao`.
- Contratações exigem tamanho de página entre `10` e `50`. Contratos funcionam
  até `500`, mínimo `10`. A biblioteca faz clamp.
- Respostas `429` e `504` são tratadas com retry e backoff de `30s`. Demais
  erros retryable usam backoff exponencial com base de `1s`.
- Escopo vazio (`204` sem conteúdo) é marcado `concluido`, com 0 registros.
- Cada UF ou município varre as 15 modalidades. Cada CNPJ varre os contratos do
  órgão. Use janelas curtas (`--data-inicial`, `--data-final`) e `--max-paginas`
  para explorar em etapas.


## Schema

**dados_livres_pncp_contrato**

- Campos tipados do `/contratos`: `orgao_cnpj`, `orgao_razao_social`,
  `codigo_ibge`, `uf_sigla`, `municipio_nome`, valores, datas, fornecedor,
  entre outros.
- `dados_json` guarda o payload cru completo.
- PK `numero_controle_pncp`. Índices em fornecedor, órgão, IBGE, UF e
  atualização.

**dados_livres_pncp_contratacao**

- Campos tipados do `/contratacoes/publicacao`: `modalidade_id/nome`,
  `objeto_compra`, `valor_total_estimado/homologado`, `situacao_compra_*`,
  `modo_disputa_*`, datas, `srp`, `emenda_parlamentar`, unidade e órgão.
- `dados_json` guarda o payload cru completo.
- PK `numero_controle_pncp`. Índices em UF, IBGE, órgão, modalidade e
  publicação.

**dados_livres_pncp_meta**

- Cobertura por escopo: `entidade`, `uf`, `municipio`, `orgao_cnpj`,
  `modalidade`, `data_inicio`, `data_fim`.
- `status` (`concluido`, `error`, `parcial`), `total_registros`,
  `ultima_pagina_ok`, `pagina_erro`.
