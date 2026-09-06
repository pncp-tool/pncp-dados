// Command harvest pagina a API de Consulta v1 do PNCP para um escopo e
// persiste os dados. Exemplos:
//
//	// contratacoes publicadas em Goiania (1 estagio, filtro server-side)
//	go run ./cmd/harvest --data-inicial 20260101 --data-final 20260105 --uf GO --municipio 5208707
//
//	// contratos de um orgao (filtra o proprio CNPJ na API)
//	go run ./cmd/harvest --data-inicial 20260101 --data-final 20260105 --cnpj-orgao 01409580000138
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/danyele/dados-livres"
	"github.com/danyele/dados-livres/internal/database"
	"github.com/danyele/dados-livres/internal/env"
	"github.com/danyele/dados-livres/pncp/types"
)

func main() {
	var (
		dataInicial string
		dataFinal   string
		ufs         string
		municipios  string
		cnpjOrgao   string
		maxPaginas  int
		tamanho     int
		storageType string
		xlsxCaminho string
		baseURL     string
		maxConcorr  int
		delayMS     int
	)
	flag.StringVar(&dataInicial, "data-inicial", "", "data inicial AAAAMMDD")
	flag.StringVar(&dataFinal, "data-final", "", "data final AAAAMMDD")
	flag.StringVar(&ufs, "uf", "", "UFs separadas por virgula (ex.: GO,DF)")
	flag.StringVar(&municipios, "municipio", "", "codigos IBGE separados por virgula (7 digitos)")
	flag.StringVar(&cnpjOrgao, "cnpj-orgao", "", "CNPJs de orgaos separados por virgula (harvesta contratos do orgao)")
	flag.IntVar(&maxPaginas, "max-paginas", 0, "limite de paginas por escopo (0 = sem limite)")
	flag.IntVar(&tamanho, "tamanho-pagina", 50, "tamanho de pagina (10..500 para contratos, 10..50 para contratacoes)")
	flag.StringVar(&storageType, "persistencia", "", "postgres ou xlsx (env DADOS_LIVRES_PERSISTENCIA)")
	flag.StringVar(&xlsxCaminho, "caminho-xlsx", "", "arquivo de saida xlsx (env DADOS_LIVRES_XLSX_CAMINHO)")
	flag.StringVar(&baseURL, "base-url", "", "URL base da API do PNCP (env DADOS_LIVRES_PNCP_BASE_URL)")
	flag.IntVar(&maxConcorr, "max-concorrencia", 0, "requisicoes simultaneas (env DADOS_LIVRES_MAX_CONCORRENCIA)")
	flag.IntVar(&delayMS, "delay-ms", 0, "intervalo minimo entre requisicoes em ms (env DADOS_LIVRES_DELAY_MS)")
	flag.Parse()

	ctx := context.Background()

	options := dadoslivres.Options{
		Storage:        storageType,
		XLSXPath:       xlsxCaminho,
		BaseURL:        env.StringOr(baseURL, types.EnvPNCPBaseURL, ""),
		MaxConcurrency: env.IntOr(maxConcorr, types.EnvMaxConcurrency, 1),
		DelayMS:        env.IntOr(delayMS, types.EnvDelayMS, 0),
		MaxPages:       env.Int(types.EnvMaxPages, 0),
	}
	if storageType == "" || storageType == types.StoragePostgres {
		cfg := database.ConfigFromEnv()
		options.Postgres = &dadoslivres.PostgresConfig{
			Host:            cfg.Host,
			Port:            cfg.Port,
			User:            cfg.User,
			Password:        cfg.Password,
			Database:        cfg.Database,
			MaxConns:        cfg.MaxConns,
			MinConns:        cfg.MinConns,
			MaxConnLifetime: cfg.MaxConnLifetime,
			MaxConnIdleTime: cfg.MaxConnIdleTime,
		}
	}

	cliente, err := dadoslivres.New(ctx, options)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro ao iniciar: %v\n", err)
		os.Exit(1)
	}
	defer cliente.Close()

	scope := types.Scope{
		StartDate:      dataInicial,
		EndDate:        dataFinal,
		States:         splitLista(ufs),
		Municipalities: splitLista(municipios),
		AgencyCNPJ:     splitLista(cnpjOrgao),
		MaxPages:       maxPaginas,
		PageSize:       tamanho,
	}

	resumo, err := cliente.Harvest(ctx, scope)
	if err != nil {
		fmt.Fprintf(os.Stderr, "erro no harvest: %v\n", err)
		os.Exit(1)
	}
	b, _ := json.MarshalIndent(resumo, "", "  ")
	fmt.Println(string(b))
}

func splitLista(v string) []string {
	partes := []string{}
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			partes = append(partes, p)
		}
	}
	return partes
}
