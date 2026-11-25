package store

// SearchFoldForTest exposes the Go fold to external tests.
func SearchFoldForTest(s string) string { return foldWord(s) }

// HomeSQLForTest exposes the home page queries (ADR-0037) to the benchmark.
func HomeSQLForTest() map[string]string {
	return map[string]string{"cards": homeCards, "list": homeList, "watching": homeWatchedSQL, "recent": homeRecentSQL}
}

func ReportsSQLForTest() map[string]string {
	return map[string]string{"kpi": reportKPISQL, "throughput": reportThroughputSQL, "approvals": reportApprovalsSQL,
		"workload": reportWorkloadSQL, "status": reportStatusSQL}
}
