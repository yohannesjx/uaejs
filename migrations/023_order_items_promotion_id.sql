-- Link order line items to applied price promotions (analytics / Promotion efficacy).
ALTER TABLE order_items
    ADD COLUMN IF NOT EXISTS promotion_id UUID REFERENCES price_promotions(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_order_items_promotion_id
    ON order_items (promotion_id)
    WHERE promotion_id IS NOT NULL;
