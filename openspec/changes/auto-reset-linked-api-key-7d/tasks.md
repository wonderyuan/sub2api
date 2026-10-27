## 1. Upstream integration

- [x] Fetch and pin upstream/main at `ba81c7fd758e425d67f856d601147ef0c5263d3f`.
- [x] Preserve the user's preexisting AGENTS.md modification and work on a separate branch.
- [x] Resolve compatibility-stream usage conflicts without losing request-body monitoring fields.
- [x] Adapt upstream route tests to the fork's unconditional step-up middleware argument.
- [x] Update the fork's Codex manifest group fixture for the new image-count billing field.
- [x] Review fork billing/admission/monitoring/audit/model-directory integration and run targeted regressions.

## 2. Account reset feature

- [x] Add the independent, default-off edit-account toggle with Chinese/English descriptions.
- [x] Preserve explicit false and unrelated Extra fields; cover standard/simple and non-OpenAI accounts.
- [x] Persist backend-managed baselines and reset cycles safely across instances.
- [x] Confirm genuine weekly rollover and atomically reset only group API key 7d usage/window.
- [x] Handle conflicting enabled sources, disabling/re-enabling, identity and membership changes.
- [x] Reuse durable cross-instance auth and billing-cache invalidation with retry.
- [x] Wire worker startup and clean shutdown; regenerate Ent/Wire as applicable.

## 3. Verification

- [x] Targeted frontend edit-modal and i18n tests.
- [x] Frontend typecheck and non-mutating lint for touched files.
- [x] Upstream merge gateway/audit/admission/inflight/monitoring/model-directory regressions.
- [x] New backend feature state-machine, transaction, configuration and cache-retry tests.
- [x] Final backend compile after worker wiring.
- [x] Final strict OpenSpec validation and complete diff/status checks.
- [ ] PostgreSQL/Redis integration tests in a dedicated disposable environment (local Docker unavailable).

Full lint/build/integration gates are not replaced by these local targeted checks. Commit and push were separately authorized by the user's GitHub submission request; deployment and production toggle changes remain excluded.
