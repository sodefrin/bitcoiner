package marketmake

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/sodefrin/bitflyer"
	"golang.org/x/sync/errgroup"
)

type config struct {
	BitflyerKey    string        `envconfig:"BITFLYER_KEY" required:"true"`
	BitflyerSecret string        `envconfig:"BITFLYER_SECRET" required:"true"`
	RiskRate       float64       `envconfig:"RISK_RATE" default:"0.3"`
	Rot            float64       `envconfig:"ROT" default:"0.01"`
	Interval       time.Duration `envconfig:"INTERVAL" default:"15s"`
}

var cfg config

func init() {
	err := envconfig.Process("", &cfg)
	if err != nil {
		envconfig.Usage("", &cfg)
		log.Fatal(err)
	}
}

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

	private, err := bf.PrivateAPIClient(cfg.BitflyerKey, cfg.BitflyerSecret)
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return realtime.Subscribe(ctx)
	})

	eg.Go(func() error {
		return m.run(ctx, realtime, private)
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func (m *MarketMake) run(ctx context.Context, realtime *bitflyer.RealtimeAPIClient, private *bitflyer.PrivateAPIClient) error {
	duration := time.Second * 5

	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			ex := realtime.GetExecutions(duration)
			mid, bids, asks := realtime.GetBoard()

			d2 := variance(ex)
			d := math.Pow(d2, 0.5)

			examount := executionAmount(ex, mid, d)
			boamount := boardAmount(bids, asks, mid, d)

			risk := 0.3

			spread := risk*d2 + 2/examount*math.Log(1+risk/boamount)

			if spread > 20000 {
				log.Print("spread is too large")
			}

			pos, err := private.GetPositions()
			if err != nil {
				return err
			}

			size := positionSize(pos)
			offset := -risk * d2 * size

			log.Printf("spread: %f, offset: %f, size: %f, SELL: %f, BUY: %f, rot: %f", spread, offset, size, mid+offset+spread/2, mid+offset-spread/2, cfg.Rot)
			eg := errgroup.Group{}

			eg.Go(func() error {
				_, err := private.CreateOrder("SELL", math.Floor(mid+offset+spread/2), cfg.Rot, "LIMIT")
				return err
			})
			eg.Go(func() error {
				_, err := private.CreateOrder("BUY", math.Floor(mid+offset-spread/2), cfg.Rot, "LIMIT")
				return err
			})

			if err := eg.Wait(); err != nil {
				return err
			}
		}
	}
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

func executionAmount(ex []*bitflyer.Execution, mid, d float64) float64 {
	amount := 0.0
	for _, v := range ex {
		if mid-d < v.Price && v.Price < mid+d {
			amount += v.Size
		}
	}
	return amount
}

func boardAmount(bids, asks []*bitflyer.Price, mid, d float64) float64 {
	amount := 0.0
	for _, v := range bids {
		if mid-d < v.Price && v.Price < mid+d {
			amount += v.Size
		}
	}
	for _, v := range asks {
		if mid-d < v.Price && v.Price < mid+d {
			amount += v.Size
		}
	}
	return amount
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
