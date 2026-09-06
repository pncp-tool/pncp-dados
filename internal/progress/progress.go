// Package progress mantem os contadores atomicos do harvest consumidos por
// progresso/SSE.
package progress

import (
	"sync/atomic"
	"time"

	"github.com/danyele/dados-livres/pncp/types"
)

// HarvestProgress agrega os contadores atomicos do harvest.
type HarvestProgress struct {
	CurrentEntity atomic.Value
	CurrentState  atomic.Value
	CurrentPage   atomic.Int64
	Contracts     atomic.Int64
	Procurements  atomic.Int64
	Pages         atomic.Int64
	Errors        atomic.Int64
	started       time.Time
}

// NewHarvestProgress cria um HarvestProgress com inicio marcado.
func NewHarvestProgress() *HarvestProgress {
	return &HarvestProgress{started: time.Now()}
}

// Event monta um ProgressEvent a partir dos contadores atuais.
func (p *HarvestProgress) Event() types.ProgressEvent {
	entity := ""
	if v := p.CurrentEntity.Load(); v != nil {
		entity = v.(string)
	}
	state := ""
	if v := p.CurrentState.Load(); v != nil {
		state = v.(string)
	}
	return types.ProgressEvent{
		CurrentEntity:   entity,
		CurrentState:    state,
		CurrentPage:     int(p.CurrentPage.Load()),
		Contracts:       int(p.Contracts.Load()),
		Procurements:    int(p.Procurements.Load()),
		Pages:           int(p.Pages.Load()),
		Errors:          int(p.Errors.Load()),
		DurationSeconds: time.Since(p.started).Seconds(),
		Timestamp:       time.Now().Format(time.RFC3339Nano),
	}
}

// SetEntity define a entidade em andamento.
func (p *HarvestProgress) SetEntity(entity string) {
	p.CurrentEntity.Store(entity)
}

// SetState define a UF em andamento.
func (p *HarvestProgress) SetState(state string) {
	p.CurrentState.Store(state)
}
