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
	RiskRate       float64       `envconfig:"RISK_RATE" default:"1"`
	RotSize        float64       `envconfig:"ROT_SIZE" default:"0.01"`
	Interval       time.Duration `envconfig:"INTERVAL" default:"30s"`
}

var cfg config
var maxRot = 10.0

func init() {
	err := envconfig.Process("", &cfg)
	if err != nil {
		envconfig.Usage("", &cfg)
		log.Fatal(err)
	}

	log.Printf("RiskRate: %f, Rot: %f, Interval: %v", cfg.RiskRate, cfg.RotSize, cfg.Interval)
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
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		return m.trade(ctx, realtime, private)
	}
}

func (m *MarketMake) trade(ctx context.Context, realtime *bitflyer.RealtimeAPIClient, private *bitflyer.PrivateAPIClient) error {
	duration := cfg.Interval
	ex := realtime.GetExecutions(duration)
	mid, bids, asks := realtime.GetBoard()

	risk := cfg.RiskRate
	d2 := variance(ex)
	d := math.Pow(d2, 0.55)

	examount := executionAmount(ex, mid, d)
	boamount := boardAmount(bids, asks, mid, d)

	if boamount == 0 {
		log.Print("boamount is zero")
	}

	spread := risk*d + 2/examount*math.Log(1+risk/boamount)

	pos, err := private.GetPositions()
	if err != nil {
		return err
	}

	size := positionSize(pos)
	offset := -risk * d * size / cfg.RotSize / maxRot

	log.Printf("d2: %f, d: %f, mid: %f, spread: %f, offset: %f, size: %f, SELL: %f, BUY: %f, rot: %f", d2, d, mid, spread, offset, size, mid+offset+spread/2, mid+offset-spread/2, cfg.RotSize)

	eg := errgroup.Group{}

	var sellID string
	var buyID string

	eg.Go(func() error {
		id, err := private.CreateOrder("SELL", math.Floor(mid+offset+spread/2), cfg.RotSize, "LIMIT")
		sellID = id
		return err
	})
	eg.Go(func() error {
		id, err := private.CreateOrder("BUY", math.Floor(mid+offset-spread/2), cfg.RotSize, "LIMIT")
		buyID = id
		return err
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	time.Sleep(duration)

	sellOrder, err := private.GetOrder(sellID)
	if err != nil {
		return err
	}
	buyOrder, err := private.GetOrder(buyID)
	if err != nil {
		return err
	}

	if sellOrder.ChildOrderState == "ACTIVE" && buyOrder.ChildOrderState == "ACTIVE" {
		if err := private.CancelOrder(sellID); err != nil {
			return err
		}
		if err := private.CancelOrder(buyID); err != nil {
			return err
		}
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
