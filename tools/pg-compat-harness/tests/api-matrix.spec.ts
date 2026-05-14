import { test, expect, APIRequestContext } from "@playwright/test";

const API = "/api/v1/fleet";

// Body markers that indicate a Postgres-driver or Postgres-syntax failure.
// Avoid bare "ERROR:" — that string appears in legitimate JSON fields too.
const PG_ERROR_MARKERS = [
  "SQLSTATE",
  "must appear in the GROUP BY",
  "operator does not exist",
  "column does not exist",
  "syntax error at or near",
  "cannot find encode plan",
  "unexpected error: pq:",
  "pgx:",
  "ERROR: relation",
  "ERROR: column",
  "ERROR: operator",
  "ERROR: function",
  "ERROR: syntax",
];

interface Probe {
  group: string;
  name: string;
  path: string;
}

async function check(request: APIRequestContext, probe: Probe) {
  const res = await request.get(probe.path);
  const status = res.status();
  let body = "";
  try {
    body = await res.text();
  } catch {
    /* ignore */
  }

  if (status === 401 || status === 403) {
    throw new Error(`auth failure (${status}) on ${probe.path} — check FLEET_TOKEN`);
  }

  const matched = PG_ERROR_MARKERS.find((m) => body.includes(m));
  expect(
    matched,
    `[${probe.group}] ${probe.name}\nGET ${probe.path}\nstatus=${status}\nbody snippet:\n${body.slice(0, 400)}`,
  ).toBeUndefined();
  expect(status, `HTTP ${status} on ${probe.path}`).toBeLessThan(500);
}

// --- Probe sets -----------------------------------------------------------

const HOST_STATUSES = ["online", "offline", "new", "mia", "missing"];
const MDM_ENROLL = ["manual", "automatic", "personal", "pending", "unenrolled", "enrolled"];
const OS_SETTINGS = ["failed", "pending", "verifying", "verified"];
const DISK_ENC = [
  "verifying",
  "verified",
  "action_required",
  "enforcing",
  "failed",
  "removing_enforcement",
];
const BOOTSTRAP = ["failed", "pending", "installed"];
const POLICY_RESPONSE = ["passing", "failing"];
const ORDER_KEYS = [
  "display_name",
  "hostname",
  "last_enrolled_at",
  "seen_time",
  "uptime",
  "memory",
  "computer_name",
  "issues",
  "primary_ip",
];
const ORDER_DIRS = ["asc", "desc"];
const PLATFORMS = ["darwin", "linux", "windows", "ios", "ipados", "android", "chrome"];

function hostProbes(): Probe[] {
  const ps: Probe[] = [];
  const push = (name: string, qs: string) =>
    ps.push({ group: "hosts", name, path: `${API}/hosts?${qs}` });

  push("baseline", "page=0&per_page=5");
  HOST_STATUSES.forEach((s) => push(`status=${s}`, `status=${s}`));
  push("low_disk_space=32", "low_disk_space=32");
  push("low_disk_space=90", "low_disk_space=90");
  push("disable_failing_policies", "disable_failing_policies=true");
  push("disable_issues", "disable_issues=true");
  push("device_mapping", "device_mapping=true");
  push("populate_software", "populate_software=true");
  push("populate_policies", "populate_policies=true");
  push("populate_users", "populate_users=true");
  push("query=ledo", "query=ledo");
  push("connected_to_fleet", "connected_to_fleet");
  MDM_ENROLL.forEach((s) => push(`mdm_enrollment_status=${s}`, `mdm_enrollment_status=${s}`));
  OS_SETTINGS.forEach((s) => push(`os_settings=${s}`, `os_settings=${s}`));
  OS_SETTINGS.forEach((s) => push(`apple_settings=${s}`, `apple_settings=${s}`));
  DISK_ENC.forEach((s) =>
    push(`os_settings_disk_encryption=${s}`, `os_settings_disk_encryption=${s}`),
  );
  DISK_ENC.forEach((s) =>
    push(`macos_settings_disk_encryption=${s}`, `macos_settings_disk_encryption=${s}`),
  );
  BOOTSTRAP.forEach((s) => push(`bootstrap_package=${s}`, `bootstrap_package=${s}`));
  ORDER_KEYS.forEach((k) =>
    ORDER_DIRS.forEach((d) =>
      push(`order_key=${k}&order_direction=${d}`, `order_key=${k}&order_direction=${d}`),
    ),
  );
  push("after=0&order_key=display_name", "after=0&order_key=display_name");
  push("team_id=0", "team_id=0");
  push("vulnerability=CVE-2007-4559", "vulnerability=CVE-2007-4559");
  return ps;
}

function hostsCountProbes(): Probe[] {
  return hostProbes().map((p) => ({
    ...p,
    group: "hosts/count",
    path: p.path.replace("/hosts?", "/hosts/count?"),
  }));
}

function softwareVersionProbes(): Probe[] {
  const ps: Probe[] = [];
  const push = (name: string, qs: string) =>
    ps.push({ group: "software/versions", name, path: `${API}/software/versions?${qs}` });

  push("baseline", "per_page=5");
  push("vulnerable=true", "vulnerable=true&per_page=5");
  push("vulnerable=true+exploit=true", "vulnerable=true&exploit=true&per_page=5");
  push("vulnerable=true+min_cvss=7", "vulnerable=true&min_cvss_score=7&per_page=5");
  push("vulnerable=true+max_cvss=5", "vulnerable=true&max_cvss_score=5&per_page=5");
  push("vulnerable=true+cvss_range", "vulnerable=true&min_cvss_score=4&max_cvss_score=9&per_page=5");
  push("query=lib", "query=lib&per_page=5");
  push("team_id=0", "team_id=0&per_page=5");
  ["name", "hosts_count", "cve_published", "cvss_score", "epss_probability"].forEach((k) =>
    ORDER_DIRS.forEach((d) =>
      push(`order_key=${k}&order_direction=${d}`, `order_key=${k}&order_direction=${d}&per_page=5`),
    ),
  );
  return ps;
}

function softwareTitleProbes(): Probe[] {
  const ps: Probe[] = [];
  const push = (name: string, qs: string) =>
    ps.push({ group: "software/titles", name, path: `${API}/software/titles?${qs}` });

  push("baseline", "per_page=5");
  push("vulnerable=true", "vulnerable=true&per_page=5");
  push("vulnerable=true+exploit=true", "vulnerable=true&exploit=true&per_page=5");
  push("available_for_install=true", "available_for_install=true&per_page=5");
  push("self_service=true", "self_service=true&per_page=5");
  push("packages_only=true", "packages_only=true&per_page=5");
  push("vulnerable=true+min_cvss=7", "vulnerable=true&min_cvss_score=7&per_page=5");
  push("query=lib", "query=lib&per_page=5");
  push("team_id=0", "team_id=0&per_page=5");
  ["name", "hosts_count"].forEach((k) =>
    ORDER_DIRS.forEach((d) =>
      push(`order_key=${k}&order_direction=${d}`, `order_key=${k}&order_direction=${d}&per_page=5`),
    ),
  );
  return ps;
}

function softwareProbes(): Probe[] {
  // deprecated /software endpoint, still served
  return softwareVersionProbes().map((p) => ({
    ...p,
    group: "software (deprecated)",
    path: p.path.replace("/software/versions?", "/software?"),
  }));
}

function vulnProbes(): Probe[] {
  const ps: Probe[] = [];
  const push = (name: string, qs: string) =>
    ps.push({ group: "vulnerabilities", name, path: `${API}/vulnerabilities?${qs}` });

  push("baseline", "per_page=5");
  push("exploit=true", "exploit=true&per_page=5");
  push("min_cvss=7", "min_cvss_score=7&per_page=5");
  push("max_cvss=5", "max_cvss_score=5&per_page=5");
  push("cvss_range", "min_cvss_score=4&max_cvss_score=9&per_page=5");
  push("query=CVE-2024", "query=CVE-2024&per_page=5");
  push("team_id=0", "team_id=0&per_page=5");
  ["cve", "cvss_score", "epss_probability", "cve_published", "hosts_count"].forEach((k) =>
    ORDER_DIRS.forEach((d) =>
      push(`order_key=${k}&order_direction=${d}`, `order_key=${k}&order_direction=${d}&per_page=5`),
    ),
  );
  return ps;
}

function dashboardProbes(): Probe[] {
  const ps: Probe[] = [];
  const push = (name: string, qs: string) =>
    ps.push({ group: "host_summary", name, path: `${API}/host_summary?${qs}` });
  push("baseline", "");
  push("low_disk_space=32", "low_disk_space=32");
  PLATFORMS.forEach((p) => push(`platform=${p}`, `platform=${p}`));
  push("team_id=0", "team_id=0");
  return ps;
}

function labelProbes(allHostsLabelId = 1): Probe[] {
  const base = `${API}/labels/${allHostsLabelId}/hosts`;
  return [
    { group: "labels/:id/hosts", name: "baseline", path: `${base}?per_page=5` },
    {
      group: "labels/:id/hosts",
      name: "status=online",
      path: `${base}?status=online&per_page=5`,
    },
    {
      group: "labels/:id/hosts",
      name: "low_disk_space=32",
      path: `${base}?low_disk_space=32&per_page=5`,
    },
  ];
}

function hostDetailProbes(hostIds: number[]): Probe[] {
  const ps: Probe[] = [];
  for (const id of hostIds) {
    ps.push({ group: "hosts/:id", name: `host ${id}`, path: `${API}/hosts/${id}` });
    ps.push({
      group: "hosts/:id/software",
      name: `host ${id}`,
      path: `${API}/hosts/${id}/software?per_page=5`,
    });
    ps.push({
      group: "hosts/:id/software",
      name: `host ${id} vulnerable=true`,
      path: `${API}/hosts/${id}/software?vulnerable=true&per_page=5`,
    });
    ps.push({
      group: "hosts/:id/policies",
      name: `host ${id}`,
      path: `${API}/hosts/${id}/policies`,
    });
    ps.push({
      group: "hosts/:id/activities",
      name: `host ${id}`,
      path: `${API}/hosts/${id}/activities?per_page=5`,
    });
    ps.push({
      group: "hosts/:id/encryption_key",
      name: `host ${id}`,
      path: `${API}/hosts/${id}/encryption_key`,
    });
  }
  return ps;
}

function miscProbes(): Probe[] {
  return [
    { group: "config", name: "config", path: `${API}/config` },
    { group: "version", name: "version", path: `${API}/version` },
    { group: "labels", name: "labels", path: `${API}/labels` },
    { group: "teams", name: "teams", path: `${API}/teams` },
    { group: "policies", name: "policies", path: `${API}/global/policies` },
    { group: "users", name: "users", path: `${API}/users` },
    { group: "sessions", name: "me", path: `${API}/me` },
    { group: "queries", name: "queries", path: `${API}/queries?per_page=5` },
    { group: "packs", name: "packs", path: `${API}/packs` },
    { group: "schedule", name: "global schedule", path: `${API}/global/schedule` },
    { group: "activities", name: "activities", path: `${API}/activities?per_page=5` },
  ];
}

// --- Dynamic discovery ----------------------------------------------------

let discoveredHostIds: number[] = [];

test.beforeAll(async ({ request }) => {
  try {
    const res = await request.get(`${API}/hosts?per_page=5`);
    if (res.ok()) {
      const data = (await res.json()) as { hosts?: Array<{ id: number }> };
      discoveredHostIds = (data.hosts ?? []).map((h) => h.id).slice(0, 3);
    }
  } catch {
    /* ignore — host detail tests will simply be skipped */
  }
});

// --- Test generation ------------------------------------------------------

function runAll(name: string, probes: Probe[]) {
  test.describe(name, () => {
    for (const probe of probes) {
      test(`${probe.group}: ${probe.name}`, async ({ request }) => {
        await check(request, probe);
      });
    }
  });
}

runAll("hosts list", hostProbes());
runAll("hosts count", hostsCountProbes());
runAll("software versions", softwareVersionProbes());
runAll("software titles", softwareTitleProbes());
runAll("software (deprecated)", softwareProbes());
runAll("vulnerabilities", vulnProbes());
runAll("dashboard / host summary", dashboardProbes());
runAll("labels", labelProbes());
runAll("misc", miscProbes());

test.describe("host detail (dynamic)", () => {
  test("host detail probes", async ({ request }) => {
    test.skip(discoveredHostIds.length === 0, "no hosts discovered");
    for (const probe of hostDetailProbes(discoveredHostIds)) {
      await check(request, probe);
    }
  });
});
