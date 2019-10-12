package marketmake

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/sodefrin/bitcoiner/config"
	"github.com/sodefrin/bitflyer"
	"golang.org/x/sync/errgroup"
)

type MarketMake struct {
	logger   *log.Logger
	realtime *bitflyer.RealtimeAPIClient
	private  *bitflyer.PrivateAPIClient

	RiskRate float64       `envconfig:"RISK_RATE" default:"1"`
	RotSize  float64       `envconfig:"ROT_SIZE" default:"0.01"`
	MaxRot   float64       `envconfig:"MAX_ROT" default:"0.06"`
	Interval time.Duration `envconfig:"INTERVAL" default:"15s"`
}

func NewMarketMake(logger *log.Logger, realtime *bitflyer.RealtimeAPIClient, private *bitflyer.PrivateAPIClient) (*MarketMake, error) {
	marketmake := &MarketMake{}

	if err := config.GetEnv(marketmake); err != nil {
		return nil, err
	}

	marketmake.logger = logger
	marketmake.realtime = realtime
	marketmake.private = private

	return marketmake, nil
}

func (m *MarketMake) Name() string {
	return "marketmake"
}

func (m *MarketMake) Execute(args []string) error {
	ctx := context.Background()

	m.logger.Printf("start %s, version: %s, RiskRate: %f, Rot: %f, Interval: %v", m.Name(), config.Version, m.RiskRate, m.RotSize, m.Interval)

	for {
		if err := m.executeCtx(ctx); err != nil {
			m.logger.Println(err)
		}
	}
}

func (m *MarketMake) executeCtx(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return m.realtime.Subscribe(ctx)
	})
	eg.Go(func() error {
		return m.run(ctx)
	})

	return eg.Wait()
}

func (m *MarketMake) run(ctx context.Context) error {
	ticker := time.NewTicker(m.Interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		if err := m.trade(ctx); err != nil {
			if err := m.private.CancelAllOrder(); err != nil {
				m.logger.Print(err)
			}
			return err
		}
	}
}

func (m *MarketMake) trade(ctx context.Context) error {
	duration := m.Interval

	pos, err := m.private.GetPositions()
	if err != nil {
		return err
	}

	size := positionSize(pos)

	ex := m.realtime.GetExecutions(duration)
	mid, _, _ := m.realtime.GetBoard()

	risk := m.RiskRate
	d2 := variance(ex)
	d := math.Pow(d2, 0.55)

	spread := risk * d

	offset := -risk * d * size / m.RotSize / m.MaxRot

	m.logger.Printf("d2: %f, d: %f, mid: %f, spread: %f, offset: %f, size: %f, SELL: %f, BUY: %f", d2, d, mid, spread, offset, size, mid+offset+spread/2, mid+offset-spread/2)

	eg := errgroup.Group{}

	eg.Go(func() error {
		price := math.Floor(mid + offset + spread/2)
		id, err := m.private.CreateOrder("SELL", price, m.RotSize, "LIMIT")
		if err != nil {
			return err
		}
		time.Sleep(m.Interval)
		m.logger.Printf("order created. sellID: %s, price: %f", id, price)
		return m.private.CancelOrder(id)
	})
	eg.Go(func() error {
		price := math.Floor(mid + offset - spread/2)
		id, err := m.private.CreateOrder("BUY", price, m.RotSize, "LIMIT")
		if err != nil {
			return err
		}
		time.Sleep(m.Interval)
		m.logger.Printf("order created. buyID: %s, price: %f", id, price)
		return m.private.CancelOrder(id)
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func variance(ex []*bitflyer.Execution) float64 {
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

func positionSize(pos []*bitflyer.Position) float64 {
	var size float64
	for _, v := range pos {
		if v.Side == "BUY" {
			size += v.Size
		} else {
			size -= v.Size
		}
	}
	return size
}
