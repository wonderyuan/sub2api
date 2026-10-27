## ADDED Requirements

### Requirement: Independent account-level opt-in

The system SHALL expose the boolean account Extra setting `auto_reset_api_keys_on_upstream_reset`, defaulting to false. It SHALL remain independent from upstream reset-credit consumption.

#### Scenario: Disabled account
- **WHEN** the setting is absent or false
- **THEN** upstream observations SHALL NOT reset any linked API key usage

#### Scenario: Enable or re-enable
- **WHEN** an administrator enables the setting for the first time or after disabling it
- **THEN** the first valid observation SHALL establish a baseline without resetting API keys

### Requirement: Confirm a real upstream weekly rollover

The system SHALL consume persisted real upstream weekly-window observations without issuing additional upstream requests. A local quota schedule or an expired unchanged snapshot SHALL NOT qualify as a confirmed rollover. Small timestamp corrections and replayed observations SHALL NOT trigger a reset.

#### Scenario: Fresh next weekly window
- **WHEN** a valid observation confirms a weekly reset beyond the established baseline
- **THEN** the system SHALL process that cycle at most once across application instances

#### Scenario: Stale observation or timestamp correction
- **WHEN** an observation is replayed, is expired, or only makes a small reset-time correction
- **THEN** the system SHALL NOT clear API key usage

### Requirement: Explicit group scope

The system SHALL select API keys through the source account's nonempty group associations. It SHALL NOT treat ungrouped API keys as belonging to an ungrouped account. A group with multiple enabled reset-source accounts SHALL be skipped with an observable diagnostic.

#### Scenario: Shared group has conflicting sources
- **WHEN** multiple accounts in a group enable the setting
- **THEN** no API keys in that group SHALL be automatically reset

### Requirement: Atomic weekly usage and window alignment

The system SHALL atomically clear target API key usage belonging to the previous weekly cycle, align `window_7d_start` to the new upstream reset time minus seven days, and persist cycle completion. Usage already accumulated in the new upstream cycle SHALL be preserved. The system SHALL preserve quota limits, cumulative quota usage, other rate-limit windows, status and expiry.

#### Scenario: Successful reset
- **WHEN** a confirmed cycle is committed
- **THEN** the 7-day usage and reset time SHALL change together and authentication/rate-limit caches SHALL be invalidated

#### Scenario: Transaction failure or duplicate instance
- **WHEN** a transaction fails or another instance processes the same cycle
- **THEN** partial resets and duplicate clearing SHALL NOT occur

### Requirement: Bounded lifecycle and private runtime state

The background worker SHALL stop cleanly with the application. Runtime baseline and cycle state SHALL be backend-managed and SHALL NOT be overwritten by ordinary client Extra payloads or copied to another account.

#### Scenario: Account copy
- **WHEN** an account is copied
- **THEN** the copy SHALL NOT inherit the original account's processed-cycle runtime state
