# pg-compat-harness

API-mode Playwright matrix that exercises every URL filter Fleet's frontend
can build against a live server, asserting each response is not a Postgres
compatibility failure (`SQLSTATE`, `must appear in the GROUP BY`,
`operator does not exist`, etc).

## Run

```sh
cd tools/pg-compat-harness
yarn install            # or: npm install / bun install
export FLEET_URL=https://fleet.hz.ledoweb.com
export FLEET_TOKEN=$(awk '/token:/ {print $2}' ~/.fleet/config)
yarn test
```

Read-only — only `GET` requests, no writes. Safe against prod.

## What it covers

- `/api/v1/fleet/hosts` and `/hosts/count`: every documented filter
  (status, low_disk_space, mdm_enrollment_status, os_settings,
  disk_encryption, bootstrap_package, policy/software/vulnerability
  filters, all order_keys × directions, populate_*, team_id, query).
- `/software/versions`, `/software/titles`, `/software` (deprecated):
  vulnerable, exploit, min/max_cvss, self_service, available_for_install,
  packages_only, team filtering, ordering.
- `/vulnerabilities`: cvss range, exploit, ordering, search.
- `/host_summary`: every platform, low_disk_space, team.
- `/labels/:id/hosts`, `/hosts/:id/*` (software/policies/activities/encryption_key).
- Sanity: `/config`, `/version`, `/labels`, `/teams`, `/me`, `/queries`,
  `/policies`, `/activities`.

## Output

`results.json` contains the full pass/fail matrix. Failing probes include
the offending URL and a 400-char body snippet, which is enough to map each
failure back to a SQL site.
