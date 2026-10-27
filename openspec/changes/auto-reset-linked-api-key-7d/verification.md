# Verification evidence

Date: 2026-10-27. Upstream target: `ba81c7fd758e425d67f856d601147ef0c5263d3f`.

## Completed upstream merge checks

Commands run from repository root; Go commands use `GOMAXPROCS=2 GOPROXY=https://goproxy.cn,direct` and `-p 1` to bound local load.

- `go -C backend build -p 1 ./...`: passed for the integrated upstream tree before new reset-worker wiring; a final compile is still required after that wiring.
- `go -C backend test -p 1 -tags=unit ./internal/handler -run 'TestGatewayCompatibilityHandlers_|TestPromptAuditGatePrecedesAccountBillingAndUpstreamSideEffects|TestGrokRealtimeBlockingGuard|TestGrokEntrypoints' -count=1`: passed. Compatibility-stream assertions also check that request-body bytes survive usage submission.
- `go -C backend test -p 1 -tags=unit ./internal/handler ./internal/service ./internal/repository -run 'RequestBodyAdmission|RequestBodyLimit|RecoveryLane|RecoveryExecution|LargeResponses|LargeAccount|ReleaseSelection|Inflight|ReserveFixedCharge|SyncBalanceCacheAfterDeduction|WrapUsageRecordTaskContext' -count=1`: passed.
- `go -C backend test -p 1 -tags=unit ./internal/service ./internal/repository ./internal/handler/admin -run 'MonitorCenter|OpsSlow|OpsInvestigation|LatencyTrend|LatencyHistogram|ConcurrencyTrend|CodexModels|OpenAIModels|PinnedModels|Grok.*Reasoning|Reasoning.*Grok|CompactRequest|CompactionRouting|SevenDay|UsageWindows|AuthInvalidation|APIKey.*7d|APIKey.*Update' -count=1`: passed.
- `go -C backend test -p 1 -tags=unit ./internal/securityaudit/... -count=1`: passed.
- `go -C backend test -p 1 -tags=unit ./internal/server/routes ./internal/server/middleware -run 'PromptAudit|PromptGuard|SecurityAudit|GrokRealtime|GrokEntrypoints|ResponsesWebSocketHas|ScopedAdmin|AlwaysStepUp' -count=1`: passed after adapting upstream test calls to the fork route signature.

## Completed frontend checks

Used existing `frontend/node_modules/.bin` directly; did not install dependencies or rewrite the lockfile.

- `vitest run --root frontend` with selected Prompt Audit, monitor-center, monitorCenter API, RateLimitOverviewPanel, CodexManifestAccountsField, feature-access, account7dWindow and i18n test paths: **25 files / 131 tests passed**.
- `vitest run --root <repo>/frontend --config <repo>/frontend/vitest.config.ts` selecting `EditAccountModal.spec.ts`, `EditAccountModal.grokUpstream.spec.ts`, `EditAccountModal.grokMediaEligibility.spec.ts`, `localeKeyCompleteness.spec.ts`, `localesMessageCompile.spec.ts`: **5 files / 111 tests passed**, including 15 new reset-toggle cases. This selection overlaps two i18n files above; counts are not an aggregate unique-test total.
- `vitest run --root <repo>/frontend --config <repo>/frontend/vitest.config.ts src/views/admin/__tests__/GroupsView.codexManifest.spec.ts --maxWorkers=1 --minWorkers=1`: **1 file / 1 test passed**.
- `frontend/node_modules/.bin/vue-tsc --noEmit --project frontend/tsconfig.json`: passed.
- Non-mutating ESLint for the edit modal, its spec, Account types, en/zh account locales and Codex manifest group fixture: passed.

## Specification and diff checks

- `OPENSPEC_TELEMETRY=0 npx --yes @fission-ai/openspec validate auto-reset-linked-api-key-7d --strict`: passed, including the final specification and implementation-evidence update.
- Strict validation of `codex-manifest-pinned-accounts` and `add-openai-compatible-prompt-audit`: passed. Structural spec validity is not a claim that all historical security/production acceptance evidence is current.
- `git diff --check`, `git diff --cached --check`: passed at completion; no unresolved merge paths remain. Following the user's explicit GitHub submission request, the upstream merge was committed as `37d566572a195b9d537dc4e7896a8ab49eec4313` on `feat/upstream-sync-key-quota-reset`; the reset feature is kept in a separate commit. The preexisting AGENTS.md modification is excluded from both commits.
- AGENTS.md preexisting working-tree blob remains `7f4982b084bfcb0276312ca2b3fe025b2091f682`.

## New backend feature verification

- `go -C backend test -p 1 ./internal/service ./internal/repository -run 'Test(AdvanceUpstreamAPIKeyReset|UpstreamAPIKeyReset|PreserveUpstreamAPIKeyReset|AuthCacheInvalidation)' -count=1`: passed for the state machine, configuration normalization, transaction rollback/outbox SQL, snapshot preservation and auth/billing-cache retry. Includes actual admin Create/Update default/true/false/nonboolean cases and unchanged-group save regression. The latest rerun after adding a 500ms implicit-lock timeout also passed.
- `go -C backend test -p 1 -tags=unit ./internal/service ./internal/repository ./internal/handler/admin ./internal/server -run 'UpstreamAPIKeyReset|AdvanceUpstreamAPIKeyReset|PreserveUpstreamAPIKeyReset|AuthCacheInvalidation|APIKey|ApiKey|AccountQuota|SchedulerCacheSnapshotMemo|SingleAccountRetry|ImageCountBilling|AccountSharedSnapshot|ModelMappingSignature|APIContracts' -count=1`: passed for all four packages. A final combined run additionally included `./internal/handler` plus compatibility streams, audit ordering, admission, inflight, monitor, model directory and protocol routing; all five packages passed after the unchanged-group guard.
- `make -C backend generate` passed; generated Wire includes worker creation, billing invalidator injection and cleanup. Ent regeneration introduced no additional drift beyond upstream generated changes. The added go.sum checksums are generation-tool dependencies, not runtime version upgrades.
- `go -C backend build -p 1 ./...` passed after new worker wiring, unchanged-group guard and implicit-lock timeout protection.
- `go -C backend test -p 1 -tags=integration -c ./internal/repository -o /tmp/sub2api-upstream-reset-repository-integration.test` passed after the final identity-switch snapshot invalidation and foreign-key lock-timeout tests. This only compiles the integration executable; it does not run PostgreSQL or Redis.
- Integration source covers multi-instance cycle idempotency and narrow field preservation, NOWAIT retry/new-cycle usage preservation, conflicting-source rebaselining, short off/on and token rotation versus identity replacement, and bounded implicit foreign-key waits. These are written and compile, not runtime-verified here.

## Pending and unexecuted

- Local Docker daemon is unavailable (`/var/run/docker.sock` missing); real PostgreSQL/Redis transaction/cache integration has not been executed. No test is allowed to use production data or shared development databases.
- Full backend lint, full integration suite, full frontend build/Vitest and GUI verification were not run locally. GitHub Actions status must be checked against the pushed commit; no CI success is implied by the local evidence above.

No production settings, credentials, balances or live API keys were modified.
