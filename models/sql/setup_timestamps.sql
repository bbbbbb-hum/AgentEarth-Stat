-- ============================================
-- 自动更新 update_time 字段的触发器脚本
-- ============================================

-- 1. 创建触发器函数（用于 update_time 字段）
CREATE OR REPLACE FUNCTION update_time_trigger()
RETURNS TRIGGER AS $$
BEGIN
    NEW.update_time = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- 2. 创建触发器函数（用于 updated_at 字段）
CREATE OR REPLACE FUNCTION updated_at_trigger()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$ language 'plpgsql';

-- ============================================
-- ae_user 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_user_time ON "public"."ae_user";
CREATE TRIGGER update_ae_user_time
BEFORE UPDATE ON "public"."ae_user"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_user_keys 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_user_keys_time ON "public"."ae_user_keys";
CREATE TRIGGER update_ae_user_keys_time
BEFORE UPDATE ON "public"."ae_user_keys"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_user_tokens 表（使用 updated_at）
-- ============================================
DROP TRIGGER IF EXISTS update_ae_user_tokens_time ON "public"."ae_user_tokens";
CREATE TRIGGER update_ae_user_tokens_time
BEFORE UPDATE ON "public"."ae_user_tokens"
FOR EACH ROW
EXECUTE FUNCTION updated_at_trigger();

-- ============================================
-- ae_mcp_services 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_mcp_services_time ON "public"."ae_mcp_services";
CREATE TRIGGER update_ae_mcp_services_time
BEFORE UPDATE ON "public"."ae_mcp_services"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_mcp_tools 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_mcp_tools_time ON "public"."ae_mcp_tools";
CREATE TRIGGER update_ae_mcp_tools_time
BEFORE UPDATE ON "public"."ae_mcp_tools"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_mcp_external_services_account 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_mcp_external_services_account_time ON "public"."ae_mcp_external_services_account";
CREATE TRIGGER update_ae_mcp_external_services_account_time
BEFORE UPDATE ON "public"."ae_mcp_external_services_account"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_mcp_external_services_config 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_mcp_external_services_config_time ON "public"."ae_mcp_external_services_config";
CREATE TRIGGER update_ae_mcp_external_services_config_time
BEFORE UPDATE ON "public"."ae_mcp_external_services_config"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_mcp_task_chain 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_mcp_task_chain_time ON "public"."ae_mcp_task_chain";
CREATE TRIGGER update_ae_mcp_task_chain_time
BEFORE UPDATE ON "public"."ae_mcp_task_chain"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- ae_mcp_task_node 表
-- ============================================
DROP TRIGGER IF EXISTS update_ae_mcp_task_node_time ON "public"."ae_mcp_task_node";
CREATE TRIGGER update_ae_mcp_task_node_time
BEFORE UPDATE ON "public"."ae_mcp_task_node"
FOR EACH ROW
EXECUTE FUNCTION update_time_trigger();

-- ============================================
-- 完成！验证触发器
-- ============================================
SELECT 
    trigger_name,
    event_object_table,
    action_timing,
    event_manipulation
FROM information_schema.triggers
WHERE trigger_schema = 'public'
  AND (trigger_name LIKE 'update_%_time' OR trigger_name LIKE 'update_%_at')
ORDER BY event_object_table;
