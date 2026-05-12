-- Admin / COGS: per-variant unit cost (store currency, typically AED).
ALTER TABLE variants
    ADD COLUMN IF NOT EXISTS unit_cost NUMERIC(19, 4);

COMMENT ON COLUMN variants.unit_cost IS 'Optional unit cost per item for inventory valuation';
