-- Reversao da migracao 002.
DROP TABLE IF EXISTS dados_livres_pncp_contratacao;

ALTER TABLE dados_livres_pncp_meta DROP CONSTRAINT IF EXISTS dados_livres_pncp_meta_escopo_key;
ALTER TABLE dados_livres_pncp_meta DROP COLUMN IF EXISTS orgao_cnpj;
ALTER TABLE dados_livres_pncp_meta ADD CONSTRAINT dados_livres_pncp_meta_escopo_key
    UNIQUE (entidade, uf, municipio, modalidade, data_inicio, data_fim);

ALTER TABLE dados_livres_pncp_contrato ADD COLUMN modalidade_id INTEGER;
ALTER TABLE dados_livres_pncp_contrato ADD COLUMN modalidade_nome TEXT;
ALTER TABLE dados_livres_pncp_contrato ADD COLUMN objeto_compra TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS idx_dados_livres_contrato_municipio_modalidade ON dados_livres_pncp_contrato (modalidade_id);