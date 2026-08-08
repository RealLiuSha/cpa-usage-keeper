package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cpa-usage-keeper/internal/auth"
	"cpa-usage-keeper/internal/ranking"
	rankinghttpapi "cpa-usage-keeper/internal/ranking/httpapi"
	servicedto "cpa-usage-keeper/internal/service/dto"
)

type shareUsageStub struct {
	overviewCalls int
	analysisCalls int
	activityCalls int
	realtimeCalls int
}

func (s *shareUsageStub) GetUsageOverview(context.Context, servicedto.UsageFilter) (*servicedto.UsageOverviewSnapshot, error) {
	s.overviewCalls++
	return &servicedto.UsageOverviewSnapshot{
		Summary: servicedto.UsageOverviewSummary{RPM: 1},
	}, nil
}

func (s *shareUsageStub) GetUsageOverviewRealtime(context.Context, servicedto.UsageFilter) (*servicedto.UsageOverviewRealtime, error) {
	s.realtimeCalls++
	return &servicedto.UsageOverviewRealtime{}, nil
}

func (s *shareUsageStub) GetUsageActivity(context.Context, servicedto.UsageFilter) (*servicedto.UsageActivitySnapshot, error) {
	s.activityCalls++
	return &servicedto.UsageActivitySnapshot{}, nil
}

func (s *shareUsageStub) GetAnalysis(context.Context, servicedto.UsageFilter) (*servicedto.AnalysisSnapshot, error) {
	s.analysisCalls++
	return &servicedto.AnalysisSnapshot{}, nil
}

func (s *shareUsageStub) GetAnalysisLatency(context.Context, servicedto.UsageFilter) (*servicedto.AnalysisLatencyDiagnostics, error) {
	return &servicedto.AnalysisLatencyDiagnostics{}, nil
}

func (s *shareUsageStub) ListUsageEvents(context.Context, servicedto.UsageFilter) (*servicedto.UsageEventsPage, error) {
	return &servicedto.UsageEventsPage{}, nil
}

func (s *shareUsageStub) StreamUsageEvents(context.Context, servicedto.UsageFilter, func(servicedto.UsageEventRecord) error) error {
	return nil
}

func (s *shareUsageStub) ListUsageEventFilterOptions(context.Context, servicedto.UsageFilter) (*servicedto.UsageEventFilterOptions, error) {
	return &servicedto.UsageEventFilterOptions{}, nil
}

type shareLocalRankingStub struct {
	calls int
}

func (s *shareLocalRankingStub) Leaderboard(context.Context, ranking.LeaderboardPeriod, ranking.LeaderboardMetric) (ranking.Leaderboard, error) {
	s.calls++
	return ranking.Leaderboard{
		Period: ranking.LeaderboardToday,
		Metric: ranking.MetricOverall,
		Entries: []ranking.LeaderboardEntry{
			{Rank: 1, ParticipantID: "1", DisplayName: "alpha", Value: 10},
		},
	}, nil
}

func (s *shareLocalRankingStub) UpdateProfile(context.Context, int64, string, uint8) (ranking.LocalProfile, error) {
	return ranking.LocalProfile{}, nil
}

func newShareTestRouter(t *testing.T, usage *shareUsageStub, local rankinghttpapi.LocalProvider) http.Handler {
	t.Helper()
	config := AuthConfig{Enabled: true, LoginPassword: "secret", SessionTTL: time.Hour}
	sessions := auth.NewSessionManager(time.Hour)
	handler := NewAuthHandler(config, sessions)
	return NewRouter(nil, statusStub{}, usage, nil, config, handler, "/keeper", OptionalProviders{
		LocalRanking:       local,
		SharePublicEnabled: true,
	})
}

func TestPublicShareRoutesAbsentWhenFeatureDisabled(t *testing.T) {
	usage := &shareUsageStub{}
	local := &shareLocalRankingStub{}
	config := AuthConfig{Enabled: true, LoginPassword: "secret", SessionTTL: time.Hour}
	sessions := auth.NewSessionManager(time.Hour)
	handler := NewAuthHandler(config, sessions)
	router := NewRouter(nil, statusStub{}, usage, nil, config, handler, "/keeper", OptionalProviders{
		LocalRanking:       local,
		SharePublicEnabled: false,
	})
	req := httptest.NewRequest(http.MethodGet, "/keeper/api/v1/public/usage/overview?range=24h", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when share public disabled, got %d body=%s", resp.Code, resp.Body.String())
	}
}

func TestPublicShareReadRoutesDoNotRequireAuth(t *testing.T) {
	usage := &shareUsageStub{}
	local := &shareLocalRankingStub{}
	router := newShareTestRouter(t, usage, local)

	for _, path := range []string{
		"/keeper/api/v1/public/usage/overview?range=24h",
		"/keeper/api/v1/public/usage/overview/realtime?window=15m",
		"/keeper/api/v1/public/usage/activity?window=day",
		"/keeper/api/v1/public/usage/analysis?range=24h",
		"/keeper/api/v1/public/usage/analysis/latency?range=24h",
		"/keeper/api/v1/public/usage/api-keys/options",
		"/keeper/api/v1/public/ranking/local/leaderboards?period=today&metric=overall",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("path %s: expected 200, got %d body=%s", path, resp.Code, resp.Body.String())
		}
	}

	if usage.overviewCalls < 1 || usage.realtimeCalls < 1 || usage.activityCalls < 1 || usage.analysisCalls < 1 {
		t.Fatalf("expected public usage handlers to run, got overview=%d realtime=%d activity=%d analysis=%d",
			usage.overviewCalls, usage.realtimeCalls, usage.activityCalls, usage.analysisCalls)
	}
	if local.calls != 1 {
		t.Fatalf("expected local ranking leaderboard once, got %d", local.calls)
	}

	// Leaderboard payload must be usable JSON (not an empty shell error).
	req := httptest.NewRequest(http.MethodGet, "/keeper/api/v1/public/ranking/local/leaderboards?period=today&metric=overall", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)
	var board ranking.Leaderboard
	if err := json.Unmarshal(resp.Body.Bytes(), &board); err != nil {
		t.Fatalf("decode leaderboard: %v body=%s", err, resp.Body.String())
	}
	if len(board.Entries) != 1 || board.Entries[0].DisplayName != "alpha" {
		t.Fatalf("unexpected leaderboard payload: %+v", board)
	}
}

func TestAdminOnlyRoutesStillRequireAuthWhenSharePublicExists(t *testing.T) {
	usage := &shareUsageStub{}
	local := &shareLocalRankingStub{}
	router := newShareTestRouter(t, usage, local)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/keeper/api/v1/usage/overview?range=24h"},
		{http.MethodGet, "/keeper/api/v1/usage/analysis?range=24h"},
		{http.MethodGet, "/keeper/api/v1/usage/api-keys/settings"},
		{http.MethodGet, "/keeper/api/v1/usage/events?range=24h"},
		{http.MethodGet, "/keeper/api/v1/ranking/local/leaderboards?period=today&metric=overall"},
		{http.MethodPatch, "/keeper/api/v1/ranking/local/profiles/1"},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.method == http.MethodPatch {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-CPA-Usage-Keeper-Request", "fetch")
			req.Body = http.NoBody
		}
		resp := httptest.NewRecorder()
		router.ServeHTTP(resp, req)
		if resp.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s: expected 401, got %d body=%s", tc.method, tc.path, resp.Code, resp.Body.String())
		}
	}
}
