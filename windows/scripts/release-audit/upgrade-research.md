# Upgrade protocol status

Protocol 2 implements staged in-place upgrade and process-interruption recovery between cooperating releases starting with 0.2.0. See [upgrade and recovery](../../docs/safe-upgrade.md). There is no network update feed, signing chain, automatic data rollback, or verified power-loss recovery.

Generation 1 safe installations and MyGo installations are refused because their application and uninstaller cannot participate in lifetime locking. The former test-upgrade-isolated.ps1 describes a historical, unimplemented side-by-side design; use test-transaction-upgrade.ps1 for the current protocol, with a current 0.2 package and a test-only 0.3 fixture.