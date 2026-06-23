-- 移除 environments.enabled 列（该字段无实际作用，发布保护由 release_frozen 承担）。
ALTER TABLE environments DROP COLUMN enabled;
