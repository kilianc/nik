package brain

import (
	"context"
)

type DataSourceOutput struct {
	Lines      []string
	Meta       map[string]string
	Processing func(ctx context.Context) error
	Processed  func(ctx context.Context) error
}

type DataSource interface {
	Check(ctx context.Context) ([]DataSourceOutput, error)
}

func (b *Brain) RegisterDataSource(source DataSource) {
	if source == nil {
		panic("register data source: nil source")
	}

	b.dataSources = append(b.dataSources, source)
}

func (b *Brain) RegisterDataSources(sources ...DataSource) {
	for _, source := range sources {
		b.RegisterDataSource(source)
	}
}
