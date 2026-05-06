DROP TRIGGER IF EXISTS trg_price_change_log_tenant ON price_change_log;
DROP TRIGGER IF EXISTS trg_price_plan_items_tenant ON price_plan_items;
DROP TRIGGER IF EXISTS trg_strategy_assignments_tenant ON strategy_assignments;

DROP FUNCTION IF EXISTS enforce_price_change_log_tenant();
DROP FUNCTION IF EXISTS enforce_price_plan_item_tenant();
DROP FUNCTION IF EXISTS enforce_strategy_assignment_tenant();
