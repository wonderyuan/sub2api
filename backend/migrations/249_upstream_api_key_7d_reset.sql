-- Account weekly-reset baselines are server-owned, not part of client extra.
CREATE TABLE IF NOT EXISTS upstream_api_key_reset_baselines (
    account_id BIGINT NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    group_id BIGINT NOT NULL REFERENCES groups(id) ON DELETE CASCADE,
    source TEXT NOT NULL,
    reset_at TIMESTAMPTZ NOT NULL,
    observed_at TIMESTAMPTZ NOT NULL,
    used_percent DOUBLE PRECISION,
    PRIMARY KEY (account_id, group_id)
);

-- Reuse the existing leased, retried, two-pass auth invalidation outbox for
-- billing rate-limit cache deletion as well. Existing events remain unchanged.
ALTER TABLE auth_cache_invalidation_outbox
    ADD COLUMN IF NOT EXISTS api_key_id BIGINT;

CREATE OR REPLACE FUNCTION clear_upstream_api_key_reset_baseline()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
DECLARE
    identity_changed BOOLEAN;
BEGIN
    IF NOT (COALESCE(OLD.extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb, false)
            OR COALESCE(NEW.extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb, false)) THEN
        RETURN NEW;
    END IF;
    identity_changed := OLD.platform IS DISTINCT FROM NEW.platform
       OR OLD.type IS DISTINCT FROM NEW.type
       OR (SELECT jsonb_object_agg(key, value) FROM jsonb_each(COALESCE(OLD.credentials, '{}'::jsonb))
           WHERE key IN ('account_id', 'chatgpt_account_id', 'chatgpt_user_id', 'user_id', 'email',
                         'organization_id', 'organization', 'org_id', 'project_id', 'base_url', 'api_key',
                         'coding_plan_provider', 'account_mode', 'zhipu_organization', 'zhipu_project'))
          IS DISTINCT FROM
          (SELECT jsonb_object_agg(key, value) FROM jsonb_each(COALESCE(NEW.credentials, '{}'::jsonb))
           WHERE key IN ('account_id', 'chatgpt_account_id', 'chatgpt_user_id', 'user_id', 'email',
                         'organization_id', 'organization', 'org_id', 'project_id', 'base_url', 'api_key',
                         'coding_plan_provider', 'account_mode', 'zhipu_organization', 'zhipu_project'))
       OR OLD.parent_account_id IS DISTINCT FROM NEW.parent_account_id;
    IF (OLD.extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb) IS DISTINCT FROM
       (NEW.extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb)
       OR identity_changed
       OR OLD.deleted_at IS DISTINCT FROM NEW.deleted_at THEN
        -- Admin writes take their row locks first. The observer only TRYs this
        -- lock and never locks accounts, avoiding billing's key -> account order.
        PERFORM pg_advisory_xact_lock(1937072737, 7);
        DELETE FROM upstream_api_key_reset_baselines WHERE account_id = OLD.id;
    END IF;
    IF identity_changed THEN
        -- A baseline cannot be rebuilt from the previous identity's snapshot.
        -- Clear the observer's exact inputs, not local quota configuration.
        -- Clearing values as well as timestamps also prevents a later partial
        -- upstream refresh from giving old weekly data a fresh timestamp.
        NEW.extra := COALESCE(NEW.extra, '{}'::jsonb) - ARRAY[
            'codex_7d_reset_at', 'codex_7d_used_percent', 'codex_usage_updated_at',
            'passive_usage_7d_reset', 'passive_usage_7d_utilization', 'passive_usage_sampled_at',
            'kimi_weekly_reset_at', 'kimi_weekly_used_percent', 'kimi_usage_updated_at',
            'zhipu_weekly_reset_at', 'zhipu_weekly_used_percent', 'zhipu_usage_updated_at',
            'minimax_weekly_reset_at', 'minimax_weekly_used_percent', 'minimax_usage_updated_at',
            'opencode_go_weekly_reset_at', 'opencode_go_weekly_used_percent', 'opencode_go_usage_updated_at',
            'command_code_weekly_reset_at', 'command_code_weekly_used_percent', 'command_code_usage_updated_at',
            'cline_weekly_reset_at', 'cline_weekly_used_percent', 'cline_usage_updated_at',
            'grok_billing_snapshot'
        ]::text[];
    END IF;
    RETURN NEW;
END;
$$;
DROP TRIGGER IF EXISTS trg_accounts_upstream_api_key_reset_baseline ON accounts;
CREATE TRIGGER trg_accounts_upstream_api_key_reset_baseline
BEFORE UPDATE OF extra, credentials, platform, type, parent_account_id, deleted_at ON accounts
FOR EACH ROW EXECUTE FUNCTION clear_upstream_api_key_reset_baseline();

CREATE OR REPLACE FUNCTION clear_upstream_api_key_reset_group_baseline()
RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM accounts
                   WHERE extra -> 'auto_reset_api_keys_on_upstream_reset' = 'true'::jsonb
                     AND id IN (CASE WHEN TG_OP <> 'INSERT' THEN OLD.account_id END,
                                CASE WHEN TG_OP <> 'DELETE' THEN NEW.account_id END)) THEN
        IF TG_OP = 'DELETE' THEN RETURN OLD; ELSE RETURN NEW; END IF;
    END IF;
    PERFORM pg_advisory_xact_lock(1937072737, 7);
    IF TG_OP <> 'INSERT' THEN
        DELETE FROM upstream_api_key_reset_baselines WHERE account_id = OLD.account_id;
    END IF;
    IF TG_OP <> 'DELETE' THEN
        DELETE FROM upstream_api_key_reset_baselines WHERE account_id = NEW.account_id;
        RETURN NEW;
    END IF;
    RETURN OLD;
END;
$$;
DROP TRIGGER IF EXISTS trg_account_groups_upstream_api_key_reset_baseline ON account_groups;
CREATE TRIGGER trg_account_groups_upstream_api_key_reset_baseline
AFTER INSERT OR UPDATE OF account_id, group_id OR DELETE ON account_groups
FOR EACH ROW EXECUTE FUNCTION clear_upstream_api_key_reset_group_baseline();
