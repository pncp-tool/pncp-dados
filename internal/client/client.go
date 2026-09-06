// Pacote client implementa o cliente HTTP de harvest da API de Consulta v1
// do PNCP (https://pncp.gov.br/api/consulta/v1), usado pelos harvests offline.
// A URL de base e interna: por padrao a oficial do PNCP, sobreponivel apenas
// via DADOS_LIVRES_PNCP_BASE_URL (testes/proxy).
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/danyele/dados-livres/internal/logger"
)

const (
	maxAttempts = 4
	backoffBase = 1 * time.Second
	backoffSlow = 30 * time.Second
	defaultBase = "https://pncp.gov.br/api/consulta/v1"
	httpTimeout = 300 * time.Second
	userAgent   = "dados-livres/1.0"
)

// Endpoints da API de Consulta v1.
const (
	// ContractsEndpoint corresponde ao endpoint /contratos (contratos assinados).
	ContractsEndpoint = "contratos"

	// ProcurementsEndpoint corresponde ao endpoint /contratacoes/publicacao
	// (contratacoes publicadas).
	ProcurementsEndpoint = "contratacoes/publicacao"
)

// Envelope e a resposta paginada da API de Consulta v1:
// { data, totalRegistros, totalPaginas, numeroPagina, paginasRestantes }.
type Envelope struct {
	TotalPages     int              `json:"totalPaginas"`
	TotalRecords   int              `json:"totalRegistros"`
	PageNumber     int              `json:"numeroPagina"`
	RemainingPages int              `json:"paginasRestantes"`
	Empty          bool             `json:"empty"`
	Data           []map[string]any `json:"data"`
}

// Client pagina a API de Consulta v1 com retries, backoff (30s para 429 e 504),
// semaforo de concorrencia e pacing minimo entre requisicoes.
type Client struct {
	baseURL string
	http    *http.Client

	sem         chan struct{}
	mu          sync.Mutex
	lastRequest time.Time
	minInterval time.Duration
}

// New cria um cliente de harvest.
func New(baseURL string, maxConcurrency, delayMS int) *Client {
	if baseURL == "" {
		baseURL = defaultBase
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 1
	}
	if delayMS < 0 {
		delayMS = 0
	}
	return &Client{
		baseURL:     baseURL,
		http:        &http.Client{Timeout: httpTimeout},
		sem:         make(chan struct{}, maxConcurrency),
		minInterval: time.Duration(delayMS) * time.Millisecond,
	}
}

// Paginate busca uma pagina de um endpoint com o mapa de filtros informado.
func (c *Client) Paginate(ctx context.Context, endpoint string, params map[string]string, page, size int) (*Envelope, error) {
	if err := c.acquire(ctx); err != nil {
		return nil, err
	}
	defer c.release()

	log := logger.New("dados-livres: client: Paginate")
	u, _ := url.Parse(c.baseURL + "/" + endpoint)
	q := u.Query()
	for k, v := range params {
		if v == "" {
			continue
		}
		q.Set(k, v)
	}
	q.Set("pagina", fmt.Sprintf("%d", page))
	q.Set("tamanhoPagina", fmt.Sprintf("%d", size))
	u.RawQuery = q.Encode()

	var lastErr error
	lastStatus := 0
	for i := 1; i <= maxAttempts; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "application/json")

		log.Info("solicitando", "endpoint", endpoint, "pagina", page, "tamanho", size, "url", u.String(), "attempt", i)
		resp, err := c.http.Do(req)
		if err != nil {
			lastErr = err
			log.Error("erro na requisicao", "attempt", i, "error", err)
		} else {
			switch {
			case resp.StatusCode == http.StatusNoContent:
				resp.Body.Close()
				log.Info("204 sem conteudo")
				return &Envelope{Empty: true}, nil
			case resp.StatusCode != http.StatusOK:
				body, _ := io.ReadAll(resp.Body)
				resp.Body.Close()
				lastStatus = resp.StatusCode
				lastErr = fmt.Errorf("pncp status %d: %s", resp.StatusCode, string(body))
				log.Error("status nao 200", "attempt", i, "error", lastErr)
				if !isRetryable(resp.StatusCode) {
					return nil, lastErr
				}
			default:
				body, rerr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if rerr != nil {
					lastErr = rerr
					log.Error("erro ao ler body", "attempt", i, "error", rerr)
				} else {
					var env Envelope
					derr := json.Unmarshal(body, &env)
					if derr == nil {
						return &env, nil
					}
					lastErr = derr
					log.Error("erro no decode", "attempt", i, "error", derr)
				}
			}
		}

		if err := waitAfterError(ctx, i, lastStatus); err != nil {
			return nil, err
		}
	}
	return nil, lastErr
}

func (c *Client) acquire(ctx context.Context) error {
	select {
	case c.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}

	if c.minInterval > 0 {
		c.mu.Lock()
		wait := time.Until(c.lastRequest.Add(c.minInterval))
		c.mu.Unlock()
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				<-c.sem
				return ctx.Err()
			}
		}
	}

	c.mu.Lock()
	c.lastRequest = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *Client) release() {
	<-c.sem
}

func isRetryable(statusCode int) bool {
	switch statusCode {
	case http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout:
		return true
	}
	return false
}

func waitWithBackoff(ctx context.Context, attempt int, base time.Duration) error {
	backoff := float64(base) * math.Pow(2, float64(attempt-1))
	jitter := (rand.Float64()*2 - 1) * backoff * 0.3
	total := time.Duration(backoff + jitter)
	if total < 0 {
		total = 0
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(total):
		return nil
	}
}

// waitAfterError aplica backoff lento (30s) para 429 (rate limit do PNCP) e 504
// (gateway), e backoff base exponencial para os demais erros retryable.
func waitAfterError(ctx context.Context, attempt, status int) error {
	switch status {
	case http.StatusTooManyRequests, http.StatusGatewayTimeout:
		return waitWithBackoff(ctx, attempt, backoffSlow)
	default:
		return waitWithBackoff(ctx, attempt, backoffBase)
	}
}
