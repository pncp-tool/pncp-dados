-- Contratacoes publicadas (endpoint /v1/contratacoes/publicacao) + limpeza:
--   - dimensao orgao_cnpj na pncp_meta (chave de cobertura de contratos por orgao);
--   - remocao das colunas de enriquecimento mortas de contratos (o /contratos
--     nunca devolve modalidade/objeto_compra; o payload cru fica em dados_json).

ALTER TABLE dados_livres_pncp_contrato DROP COLUMN IF EXISTS modalidade_id;
ALTER TABLE dados_livres_pncp_contrato DROP COLUMN IF EXISTS modalidade_nome;
ALTER TABLE dados_livres_pncp_contrato DROP COLUMN IF EXISTS objeto_compra;
DROP INDEX IF EXISTS idx_dados_livres_contrato_municipio_modalidade;

ALTER TABLE dados_livres_pncp_meta ADD COLUMN IF NOT EXISTS orgao_cnpj TEXT NOT NULL DEFAULT '';
ALTER TABLE dados_livres_pncp_meta DROP CONSTRAINT IF EXISTS dados_livres_pncp_meta_escopo_key;
ALTER TABLE dados_livres_pncp_meta ADD CONSTRAINT dados_livres_pncp_meta_escopo_key
    UNIQUE (entidade, uf, municipio, modalidade, orgao_cnpj, data_inicio, data_fim);

CREATE TABLE IF NOT EXISTS dados_livres_pncp_contratacao (
    numero_controle_pncp TEXT NOT NULL,
    numero_compra TEXT,
    ano_compra INTEGER,
    sequencial_compra INTEGER,
    modalidade_id INTEGER,
    modalidade_nome TEXT,
    modo_disputa_id INTEGER,
    modo_disputa_nome TEXT,
    situacao_compra_id INTEGER,
    situacao_compra_nome TEXT,
    objeto_compra TEXT NOT NULL DEFAULT '',
    valor_total_estimado NUMERIC(18, 2),
    valor_total_homologado NUMERIC(18, 2),
    data_publicacao_pncp TIMESTAMPTZ,
    data_inclusao TIMESTAMPTZ,
    data_atualizacao TIMESTAMPTZ,
    data_atualizacao_global TIMESTAMPTZ,
    data_abertura_proposta TIMESTAMPTZ,
    data_encerramento_proposta TIMESTAMPTZ,
    orgao_cnpj TEXT,
    orgao_razao_social TEXT,
    codigo_ibge TEXT,
    municipio_nome TEXT,
    uf_sigla TEXT,
    unidade_nome TEXT,
    tipo_instrumento_convocatorio_codigo INTEGER,
    tipo_instrumento_convocatorio_nome TEXT,
    srp BOOLEAN,
    emenda_parlamentar BOOLEAN,
    processo TEXT,
    link_processo_eletronico TEXT,
    link_sistema_origem TEXT,
    usuario_nome TEXT,
    dados_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT dados_livres_pncp_contratacao_pkey PRIMARY KEY (numero_controle_pncp)
);

CREATE INDEX IF NOT EXISTS idx_dados_livres_contratacao_uf ON dados_livres_pncp_contratacao (uf_sigla);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contratacao_ibge ON dados_livres_pncp_contratacao (codigo_ibge);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contratacao_orgao ON dados_livres_pncp_contratacao (orgao_cnpj);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contratacao_modalidade ON dados_livres_pncp_contratacao (modalidade_id);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contratacao_publicacao ON dados_livres_pncp_contratacao (data_publicacao_pncp);