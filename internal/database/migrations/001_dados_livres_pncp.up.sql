-- Tabelas da biblioteca dados-livres (prefixo dados_livres_*) para evitar
-- colisao com o schema do oicp (pncp_contrato / pncp_meta).
--
-- Espelha exatamente o schema atual do oicp (verificado via pg_dump em
-- tse_data): todos os campos tipados do /contratos sao persistidos, e o payload
-- cru completo da API e preservado em dados_json.

CREATE TABLE IF NOT EXISTS dados_livres_pncp_contrato (
    numero_controle_pncp TEXT NOT NULL,
    numero_controle_pncp_compra TEXT,
    ano_contrato INTEGER,
    sequencial_contrato INTEGER,
    objeto_contrato TEXT NOT NULL DEFAULT '',
    ni_fornecedor TEXT,
    tipo_pessoa TEXT,
    nome_razao_social_fornecedor TEXT NOT NULL DEFAULT '',
    ni_fornecedor_sub_contratado TEXT,
    nome_fornecedor_sub_contratado TEXT,
    valor_inicial NUMERIC(18, 2),
    valor_global NUMERIC(18, 2),
    valor_acumulado NUMERIC(18, 2),
    data_assinatura TIMESTAMPTZ,
    data_vigencia_inicio TIMESTAMPTZ,
    data_vigencia_fim TIMESTAMPTZ,
    data_publicacao_pncp TIMESTAMPTZ,
    data_atualizacao_global TIMESTAMPTZ,
    orgao_cnpj TEXT,
    orgao_razao_social TEXT,
    codigo_ibge TEXT,
    uf_sigla TEXT,
    tipo_contrato TEXT,
    municipio_nome TEXT,
    modalidade_id INTEGER,
    modalidade_nome TEXT,
    objeto_compra TEXT NOT NULL DEFAULT '',
    dados_json JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT dados_livres_pncp_contrato_pkey PRIMARY KEY (numero_controle_pncp)
);

CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_fornecedor ON dados_livres_pncp_contrato (ni_fornecedor);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_orgao ON dados_livres_pncp_contrato (orgao_cnpj);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_ibge ON dados_livres_pncp_contrato (codigo_ibge);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_uf ON dados_livres_pncp_contrato (uf_sigla);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_compra ON dados_livres_pncp_contrato (numero_controle_pncp_compra);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_atualizacao ON dados_livres_pncp_contrato (data_atualizacao_global);
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_municipio_modalidade ON dados_livres_pncp_contrato (modalidade_id);

CREATE TABLE IF NOT EXISTS dados_livres_pncp_meta (
    id BIGSERIAL PRIMARY KEY,
    entidade TEXT NOT NULL,
    uf TEXT NOT NULL DEFAULT '',
    municipio TEXT NOT NULL DEFAULT '',
    data_inicio TEXT NOT NULL,
    data_fim TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'concluido' CHECK (status IN ('concluido', 'error', 'parcial')),
    total_registros INTEGER,
    atualizado_em TIMESTAMPTZ,
    modalidade INTEGER NOT NULL DEFAULT 0,
    ultima_pagina_ok INTEGER NOT NULL DEFAULT 0,
    pagina_erro INTEGER,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT dados_livres_pncp_meta_escopo_key UNIQUE (entidade, uf, municipio, modalidade, data_inicio, data_fim)
);