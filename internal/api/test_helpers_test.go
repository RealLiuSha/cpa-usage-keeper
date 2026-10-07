package api

import (
	"context"

	servicedto "cpa-usage-keeper/internal/service/dto"
)

type usageFilterStub struct {
	overview      *servicedto.UsageOverviewSnapshot
	realtime      *servicedto.UsageOverviewRealtime
	err           error
	lastFilter    servicedto.UsageFilter
	lastRealtime  servicedto.UsageFilter
	overviewCalls int
	realtimeCalls int
}

func (s *usageFilterStub) GetUsageOverview(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageOverviewSnapshot, error) {
	s.lastFilter = filter
	s.overviewCalls++
	return s.overview, s.err
}

func (s *usageFilterStub) GetUsageActivity(context.Context, servicedto.UsageFilter) (*servicedto.UsageActivitySnapshot, error) {
	return nil, s.err
}

func (s *usageFilterStub) GetUsageOverviewRealtime(_ context.Context, filter servicedto.UsageFilter) (*servicedto.UsageOverviewRealtime, error) {
	s.lastRealtime = filter
	s.realtimeCalls++
	return s.realtime, s.err
}

func (s *usageFilterStub) ListUsageEvents(context.Context, servicedto.UsageFilter) (*servicedto.UsageEventsPage, error) {
	return nil, s.err
}

func (s *usageFilterStub) StreamUsageEvents(context.Context, servicedto.UsageFilter, func(servicedto.UsageEventRecord) error) error {
	return s.err
}

func (s *usageFilterStub) ListUsageEventFilterOptions(context.Context, servicedto.UsageFilter) (*servicedto.UsageEventFilterOptions, error) {
	return nil, s.err
}

func (s *usageFilterStub) GetAnalysis(context.Context, servicedto.UsageFilter) (*servicedto.AnalysisSnapshot, error) {
	return nil, s.err
}

func (s *usageFilterStub) GetAnalysisLatency(context.Context, servicedto.UsageFilter) (*servicedto.AnalysisLatencyDiagnostics, error) {
	return nil, s.err
}
