# Repository planning

The authoritative planning file is [planning.json](planning.json) on the `projects-overview-planning` branch. The Projects Overview manager reads it when connecting or syncing this repository.

Horizon: unplanned (empty), now, next, later, blocked. Effort: unestimated (empty), xs, s, m, l, xl.

Entries use stable repository and GitHub issue IDs; issue numbers are included for readers. Each entry has a revision and UTC update time. Clearing values retains a revision tombstone. Removing the repository from the manager does not delete this file. Pending local changes are not saved until a GitHub commit is confirmed.

Use the manager to edit planning. Concurrent changes are checked before saving. This branch does not change issue labels/types or the code branch. Actual deletion of the GitHub repository requires a backup for recovery.
