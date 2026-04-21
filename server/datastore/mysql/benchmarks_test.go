package mysql

// MySQL vs PostgreSQL performance benchmarks.
//
// Run against MySQL:
//
//	MYSQL_TEST=1 go test -bench=Benchmark -benchtime=5s -count=5 -run=^$ ./server/datastore/mysql/ > /tmp/mysql.bench
//
// Run against PostgreSQL (requires postgres_test container on port 5434):
//
//	POSTGRES_TEST=1 go test -bench=Benchmark -benchtime=5s -count=5 -run=^$ ./server/datastore/mysql/ > /tmp/pg.bench
//
// Compare:
//
//	go install golang.org/x/perf/cmd/benchstat@latest
//	benchstat /tmp/mysql.bench /tmp/pg.bench

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/fleetdm/fleet/v4/server/fleet"
	"github.com/fleetdm/fleet/v4/server/test"
)

// BenchmarkUpdateHostSoftware measures the hot path that runs once per hour per host.
// It simulates a host reporting 100 installed packages with one version change per iteration.
func BenchmarkUpdateHostSoftware(b *testing.B) {
	ds := CreateDS(b)
	ctx := context.Background()

	host := test.NewHost(b, ds, "bench-host", "1.2.3.4", "bench-key", "bench-uuid-sw", time.Now())

	sw := make([]fleet.Software, 100)
	for i := range sw {
		sw[i] = fleet.Software{
			Name:    fmt.Sprintf("pkg-%03d", i),
			Version: "1.0.0",
			Source:  "deb_packages",
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sw[0].Version = fmt.Sprintf("1.0.%d", i) // simulate one package updating each run
		if _, err := ds.UpdateHostSoftware(ctx, host.ID, sw); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListSoftware measures the goqu-based query path with multiple JOINs.
// 50 distinct software items are seeded via UpdateHostSoftware; software_host_counts
// is populated directly (avoiding the slow SyncHostsSoftware table-swap).
func BenchmarkListSoftware(b *testing.B) {
	ds := CreateDS(b)
	ctx := context.Background()

	host := test.NewHost(b, ds, "bench-sw-host", "10.0.0.1", "bench-sw-key", "bench-sw-uuid", time.Now())
	sw := make([]fleet.Software, 50)
	for i := range sw {
		sw[i] = fleet.Software{
			Name:    fmt.Sprintf("pkg-%03d", i),
			Version: "1.0.0",
			Source:  "deb_packages",
		}
	}
	if _, err := ds.UpdateHostSoftware(ctx, host.ID, sw); err != nil {
		b.Fatal(err)
	}

	// Seed software_host_counts directly — SyncHostsSoftware does an atomic table swap
	// that is too slow for benchmark setup.
	// global_stats=true/1 means these are the global (cross-team) counts.
	_, err := ds.writer(ctx).ExecContext(ctx, `
		INSERT INTO software_host_counts (software_id, hosts_count, team_id, global_stats, updated_at)
		SELECT hs.software_id, 1, 0, ?, NOW() FROM host_software hs WHERE hs.host_id = ?
	`, true, host.ID)
	if err != nil {
		b.Fatal(err)
	}

	opts := fleet.SoftwareListOptions{
		ListOptions: fleet.ListOptions{
			PerPage:         25,
			OrderKey:        "name",
			IncludeMetadata: true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, err := ds.ListSoftware(ctx, opts); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkListHosts measures the 6+ LEFT JOIN host listing query, the Fleet UI's main hot path.
// 200 hosts are seeded; the benchmark fetches the first page of 25.
func BenchmarkListHosts(b *testing.B) {
	ds := CreateDS(b)
	ctx := context.Background()

	const nHosts = 200

	now := time.Now()
	for i := 0; i < nHosts; i++ {
		test.NewHost(b, ds,
			fmt.Sprintf("bench-host-%d", i),
			fmt.Sprintf("10.1.0.%d", i%254+1),
			fmt.Sprintf("bench-key-%d", i),
			fmt.Sprintf("bench-uuid-%d", i),
			now,
		)
	}

	filter := fleet.TeamFilter{IncludeObserver: true}
	opts := fleet.HostListOptions{
		ListOptions: fleet.ListOptions{PerPage: 25},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ds.ListHosts(ctx, filter, opts); err != nil {
			b.Fatal(err)
		}
	}
}
