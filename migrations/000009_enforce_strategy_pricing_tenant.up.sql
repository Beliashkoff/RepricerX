DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM strategy_assignments sa
        JOIN strategies st ON st.id = sa.strategy_id
        JOIN products p ON p.id = sa.product_id
        JOIN shops sh ON sh.id = p.shop_id
        WHERE sa.product_id IS NOT NULL
          AND st.user_id <> sh.user_id
    ) THEN
        RAISE EXCEPTION 'strategy_assignments contains cross-tenant strategy/product links'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'strategy_assignments_tenant_check';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM price_plan_items ppi
        JOIN price_plans pp ON pp.id = ppi.plan_id
        JOIN products p ON p.id = ppi.product_id
        WHERE pp.shop_id <> p.shop_id
    ) THEN
        RAISE EXCEPTION 'price_plan_items contains cross-shop plan/product links'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'price_plan_items_product_tenant_check';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM price_plan_items ppi
        JOIN price_plans pp ON pp.id = ppi.plan_id
        JOIN shops sh ON sh.id = pp.shop_id
        JOIN strategies st ON st.id = ppi.strategy_id
        WHERE ppi.strategy_id IS NOT NULL
          AND st.user_id <> sh.user_id
    ) THEN
        RAISE EXCEPTION 'price_plan_items contains cross-tenant plan/strategy links'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'price_plan_items_strategy_tenant_check';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM price_change_log pcl
        JOIN products p ON p.id = pcl.product_id
        WHERE pcl.shop_id <> p.shop_id
    ) THEN
        RAISE EXCEPTION 'price_change_log contains cross-shop shop/product links'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'price_change_log_product_tenant_check';
    END IF;

    IF EXISTS (
        SELECT 1
        FROM price_change_log pcl
        JOIN shops sh ON sh.id = pcl.shop_id
        JOIN strategies st ON st.id = pcl.strategy_id
        WHERE pcl.strategy_id IS NOT NULL
          AND st.user_id <> sh.user_id
    ) THEN
        RAISE EXCEPTION 'price_change_log contains cross-tenant shop/strategy links'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'price_change_log_strategy_tenant_check';
    END IF;
END $$;

CREATE OR REPLACE FUNCTION enforce_strategy_assignment_tenant()
RETURNS trigger AS $$
DECLARE
    strategy_user_id UUID;
    product_user_id UUID;
BEGIN
    IF NEW.product_id IS NULL THEN
        RETURN NEW;
    END IF;

    SELECT user_id
    INTO strategy_user_id
    FROM strategies
    WHERE id = NEW.strategy_id;

    SELECT sh.user_id
    INTO product_user_id
    FROM products p
    JOIN shops sh ON sh.id = p.shop_id
    WHERE p.id = NEW.product_id;

    IF strategy_user_id IS NULL OR product_user_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF strategy_user_id <> product_user_id THEN
        RAISE EXCEPTION 'strategy assignment links strategy and product from different tenants'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'strategy_assignments_tenant_check';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_price_plan_item_tenant()
RETURNS trigger AS $$
DECLARE
    plan_shop_id UUID;
    plan_user_id UUID;
    product_shop_id UUID;
    strategy_user_id UUID;
BEGIN
    SELECT pp.shop_id, sh.user_id
    INTO plan_shop_id, plan_user_id
    FROM price_plans pp
    JOIN shops sh ON sh.id = pp.shop_id
    WHERE pp.id = NEW.plan_id;

    SELECT shop_id
    INTO product_shop_id
    FROM products
    WHERE id = NEW.product_id;

    IF plan_shop_id IS NULL OR product_shop_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF plan_shop_id <> product_shop_id THEN
        RAISE EXCEPTION 'price plan item links plan and product from different shops'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'price_plan_items_product_tenant_check';
    END IF;

    IF NEW.strategy_id IS NOT NULL THEN
        SELECT user_id
        INTO strategy_user_id
        FROM strategies
        WHERE id = NEW.strategy_id;

        IF strategy_user_id IS NOT NULL AND strategy_user_id <> plan_user_id THEN
            RAISE EXCEPTION 'price plan item links plan and strategy from different tenants'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'price_plan_items_strategy_tenant_check';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION enforce_price_change_log_tenant()
RETURNS trigger AS $$
DECLARE
    log_user_id UUID;
    product_shop_id UUID;
    strategy_user_id UUID;
BEGIN
    SELECT user_id
    INTO log_user_id
    FROM shops
    WHERE id = NEW.shop_id;

    SELECT shop_id
    INTO product_shop_id
    FROM products
    WHERE id = NEW.product_id;

    IF log_user_id IS NULL OR product_shop_id IS NULL THEN
        RETURN NEW;
    END IF;

    IF NEW.shop_id <> product_shop_id THEN
        RAISE EXCEPTION 'price change log links shop and product from different shops'
            USING ERRCODE = '23514',
                  CONSTRAINT = 'price_change_log_product_tenant_check';
    END IF;

    IF NEW.strategy_id IS NOT NULL THEN
        SELECT user_id
        INTO strategy_user_id
        FROM strategies
        WHERE id = NEW.strategy_id;

        IF strategy_user_id IS NOT NULL AND strategy_user_id <> log_user_id THEN
            RAISE EXCEPTION 'price change log links shop and strategy from different tenants'
                USING ERRCODE = '23514',
                      CONSTRAINT = 'price_change_log_strategy_tenant_check';
        END IF;
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_strategy_assignments_tenant ON strategy_assignments;
CREATE TRIGGER trg_strategy_assignments_tenant
    BEFORE INSERT OR UPDATE OF strategy_id, product_id
    ON strategy_assignments
    FOR EACH ROW
    EXECUTE FUNCTION enforce_strategy_assignment_tenant();

DROP TRIGGER IF EXISTS trg_price_plan_items_tenant ON price_plan_items;
CREATE TRIGGER trg_price_plan_items_tenant
    BEFORE INSERT OR UPDATE OF plan_id, product_id, strategy_id
    ON price_plan_items
    FOR EACH ROW
    EXECUTE FUNCTION enforce_price_plan_item_tenant();

DROP TRIGGER IF EXISTS trg_price_change_log_tenant ON price_change_log;
CREATE TRIGGER trg_price_change_log_tenant
    BEFORE INSERT OR UPDATE OF shop_id, product_id, strategy_id
    ON price_change_log
    FOR EACH ROW
    EXECUTE FUNCTION enforce_price_change_log_tenant();
