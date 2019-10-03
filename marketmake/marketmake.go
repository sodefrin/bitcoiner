package marketmake

import (
	"context"
	"fmt"
	"time"

	"github.com/sodefrin/bitflyer"
	"golang.org/x/sync/errgroup"
)

type MarketMake struct {
}

func NewMarketMake() *MarketMake {
	return &MarketMake{}
}

func (m *MarketMake) Name() string {
	return "marketmake"
}

func (m *MarketMake) Execute(args []string) error {
	ctx := context.Background()

	bf := bitflyer.NewBitflyer()

	realtime, err := bf.GetRealtimeAPIClient()
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return realtime.Subscribe(ctx)
	})

	eg.Go(func() error {
		return m.run(ctx, realtime)
	})

	if err := eg.Wait(); err != nil {
		fmt.Println(err)
	}

	return nil
}

func (m *MarketMake) run(ctx context.Context, realtime *bitflyer.RealtimeAPIClient) error {
	duration := time.Second * 10

	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			ex := realtime.GetExecutions(duration)
			fmt.Println(m.variance(ex))
		}
	}
}

func (m *MarketMake) variance(ex []*bitflyer.Execution) float64 {
	sum := 0.0
	sum2 := 0.0
	n := 0.0
	for _, v := range ex {
		sum += v.Price
		sum2 += v.Price * v.Price
		n++
	}
	return sum2/n - (sum/n)*(sum/n)
}
