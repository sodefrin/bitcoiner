package marketmake

import (
	"context"
	"fmt"
	"log"
	"math"
	"time"

	"cloud.google.com/go/logging"
	monitoring "cloud.google.com/go/monitoring/apiv3"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/kelseyhightower/envconfig"
	"github.com/sodefrin/bitflyer"
	"golang.org/x/sync/errgroup"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

type config struct {
	ProjectID      string        `envconfig:"PROJECT_ID" required:"true"`
	BitflyerKey    string        `envconfig:"BITFLYER_KEY" required:"true"`
	BitflyerSecret string        `envconfig:"BITFLYER_SECRET" required:"true"`
	RiskRate       float64       `envconfig:"RISK_RATE" default:"1"`
	RotSize        float64       `envconfig:"ROT_SIZE" default:"0.01"`
	Interval       time.Duration `envconfig:"INTERVAL" default:"15s"`
}

var cfg config
var maxRot = 4.0
var logger *log.Logger

func init() {
	err := envconfig.Process("", &cfg)
	if err != nil {
		envconfig.Usage("", &cfg)
		panic(err)
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

	client, err := logging.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		return err
	}
	defer client.Close()

	logger = client.Logger(m.Name()).StandardLogger(logging.Info)
	logger.Printf("RiskRate: %f, Rot: %f, Interval: %v", cfg.RiskRate, cfg.RotSize, cfg.Interval)

	for {
		if err := m.executeCtx(ctx); err != nil {
			logger.Println(err)
		}
	}
}

func (m *MarketMake) executeCtx(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	bf := bitflyer.NewBitflyer()

	realtime, err := bf.GetRealtimeAPIClient()
	if err != nil {
		return err
	}

	private, err := bf.PrivateAPIClient(cfg.BitflyerKey, cfg.BitflyerSecret)
	if err != nil {
		return err
	}

	eg.Go(func() error {
		return realtime.Subscribe(ctx)
	})

	eg.Go(func() error {
		return m.run(ctx, realtime, private)
	})

	eg.Go(func() error {
		return m.trace(ctx)
	})

	return eg.Wait()
}

func (m *MarketMake) run(ctx context.Context, realtime *bitflyer.RealtimeAPIClient, private *bitflyer.PrivateAPIClient) error {
	ticker := time.NewTicker(cfg.Interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		if err := m.trade(ctx, realtime, private); err != nil {
			if err := private.CancelAllOrder(); err != nil {
				logger.Print(err)
			}
			return err
		}
	}
}

func (m *MarketMake) trade(ctx context.Context, realtime *bitflyer.RealtimeAPIClient, private *bitflyer.PrivateAPIClient) error {
	duration := cfg.Interval

	pos, err := private.GetPositions()
	if err != nil {
		return err
	}

	size := positionSize(pos)

	ex := realtime.GetExecutions(duration)
	mid, _, _ := realtime.GetBoard()

	risk := cfg.RiskRate
	d2 := variance(ex)
	d := math.Pow(d2, 0.55)

	spread := risk * d

	offset := -risk * d * size / cfg.RotSize / maxRot

	logger.Printf("d2: %f, d: %f, mid: %f, spread: %f, offset: %f, size: %f, SELL: %f, BUY: %f, rot: %f", d2, d, mid, spread, offset, size, mid+offset+spread/2, mid+offset-spread/2, cfg.RotSize)

	eg := errgroup.Group{}

	var sellID string
	var buyID string

	eg.Go(func() error {
		id, err := private.CreateOrder("SELL", math.Floor(mid+offset+spread/2), cfg.RotSize, "LIMIT")
		if err != nil {
			return err
		}
		time.Sleep(cfg.Interval)
		return private.CancelOrder(id)
	})
	eg.Go(func() error {
		id, err := private.CreateOrder("BUY", math.Floor(mid+offset-spread/2), cfg.RotSize, "LIMIT")
		if err != nil {
			return err
		}
		time.Sleep(cfg.Interval)
		return private.CancelOrder(id)
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	logger.Printf("order created. sellID: %s, buyID: %s", sellID, buyID)
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

func (m *MarketMake) trace(ctx context.Context) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		<-ticker.C
		if err := traceCollateral(ctx, 1.0); err != nil {
			return nil
		}
	}
}

func traceCollateral(ctx context.Context, value float64) error {
	c, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return err
	}
	now := &timestamp.Timestamp{
		Seconds: time.Now().Unix(),
	}
	req := &monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + cfg.ProjectID,
		TimeSeries: []*monitoringpb.TimeSeries{{
			Metric: &metricpb.Metric{
				Type: "custom.googleapis.com/collateral",
			},
			Points: []*monitoringpb.Point{{
				Interval: &monitoringpb.TimeInterval{
					EndTime: now,
				},
				Value: &monitoringpb.TypedValue{
					Value: &monitoringpb.TypedValue_DoubleValue{
						DoubleValue: value,
					},
				},
			}},
		}},
	}
	log.Printf("writeTimeseriesRequest: %+v\n", req)

	err = c.CreateTimeSeries(ctx, req)
	if err != nil {
		return fmt.Errorf("could not write time series value, %v ", err)
	}
	return nil
}
