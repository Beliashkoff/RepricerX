-- Migration 000008: drop legacy marketplace-specific tables.
-- PREREQUISITE: run cmd/credbackfill and verify failed=0 before applying this migration.
-- All four DROP statements are idempotent (IF EXISTS) so it is safe to apply even if
-- migration 000003 already dropped them on a previous installation.

DROP TABLE IF EXISTS competitors_ozon;
DROP TABLE IF EXISTS products_ozon;
DROP TABLE IF EXISTS shops_ozon;
DROP TABLE IF EXISTS competitors_wb;
DROP TABLE IF EXISTS products_wb;
DROP TABLE IF EXISTS shops_wb;
DROP TABLE IF EXISTS payments;
