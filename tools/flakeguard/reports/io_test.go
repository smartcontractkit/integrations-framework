package reports

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	splunkToken   = "test-token"
	splunkEvent   = "test"
	reportID      = "123"
	totalTestRuns = 270
	testRunCount  = 15
	uniqueTests   = 19
)

func TestAggregateResultFiles(t *testing.T) {
	t.Parallel()

	report, err := LoadAndAggregate("./testdata", WithReportID(reportID))
	require.NoError(t, err, "LoadAndAggregate failed")
	verifyAggregatedReport(t, report)
}

func verifyAggregatedReport(t *testing.T, report *TestReport) {
	require.NotNil(t, report, "report is nil")
	require.Equal(t, reportID, report.ID, "report ID mismatch")
	require.Equal(t, uniqueTests, len(report.Results), "report results count mismatch")
	require.Equal(t, totalTestRuns, report.SummaryData.TotalRuns, "report test total runs mismatch")
	require.Equal(t, false, report.RaceDetection, "race detection should be false")

	var (
		testFail, testSkipped, testPass TestResult
		testFailName                    = "TestFail"
		testSkippedName                 = "TestSkipped"
		testPassName                    = "TestPass"
	)
	for _, result := range report.Results {
		if result.TestName == testFailName {
			testFail = result
		}
		if result.TestName == testSkippedName {
			testSkipped = result
		}
		if result.TestName == testPassName {
			testPass = result
		}
	}

	t.Run("verify TestFail", func(t *testing.T) {
		require.Equal(t, testFailName, testFail.TestName, "TestFail not found")
		assert.False(t, testFail.Panic, "TestFail should not panic")
		assert.False(t, testFail.Skipped, "TestFail should not be skipped")
		assert.Equal(t, testRunCount, testFail.Runs, "TestFail should run every time")
		assert.Zero(t, testFail.Skips, "TestFail should not be skipped")
		assert.Equal(t, testRunCount, testFail.Failures, "TestFail should fail every time")
		assert.Len(t, testFail.Durations, testRunCount, "TestFail should have durations")
	})

	t.Run("verify TestSkipped", func(t *testing.T) {
		require.Equal(t, testSkippedName, testSkipped.TestName, "TestSkip not found")
		assert.False(t, testSkipped.Panic, "TestSkipped should not panic")
		assert.Zero(t, testSkipped.Runs, "TestSkipped should not pass")
		assert.True(t, testSkipped.Skipped, "TestSkipped should be skipped")
		assert.Equal(t, testRunCount, testSkipped.Skips, "TestSkipped should be skipped entirely")
		assert.Empty(t, testSkipped.Durations, "TestSkipped should not have durations")
	})

	t.Run("verify TestPass", func(t *testing.T) {
		require.Equal(t, testPassName, testPass.TestName, "TestPass not found")
		assert.False(t, testPass.Panic, "TestPass should not panic")
		assert.Equal(t, testRunCount, testPass.Runs, "TestPass should run every time")
		assert.False(t, testPass.Skipped, "TestPass should not be skipped")
		assert.Zero(t, testPass.Skips, "TestPass should not be skipped")
		assert.Equal(t, testRunCount, testPass.Successes, "TestPass should pass every time")
		assert.Len(t, testPass.Durations, testRunCount, "TestPass should have durations")
	})
}

func BenchmarkAggregateResultFiles(b *testing.B) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	for i := 0; i < b.N; i++ {
		_, err := LoadAndAggregate("./testdata", WithReportID(reportID))
		require.NoError(b, err, "LoadAndAggregate failed")
	}
}

func BenchmarkAggregateResultFilesSplunk(b *testing.B) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	b.Cleanup(srv.Close)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := LoadAndAggregate("./testdata", WithReportID(reportID), WithSplunk(srv.URL, splunkToken, "test"))
		require.NoError(b, err, "LoadAndAggregate failed")
	}
}
