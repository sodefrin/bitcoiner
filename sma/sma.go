package sma

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/sodefrin/bitcoiner/config"
	"github.com/sodefrin/bitflyer"
	"golang.org/x/sync/errgroup"
)

type Sma struct {
	logger   *log.Logger
	realtime *bitflyer.RealtimeAPIClient
	private  *bitflyer.PrivateAPIClient

	RiskRate float64       `envconfig:"RISK_RATE" default:"1"`
	RotSize  float64       `envconfig:"ROT_SIZE" default:"0.01"`
	MaxRot   float64       `envconfig:"MAX_ROT" default:"5"`
	Interval time.Duration `envconfig:"INTERVAL" default:"15s"`
}

func NewSma(logger *log.Logger, realtime *bitflyer.RealtimeAPIClient, private *bitflyer.PrivateAPIClient) (*Sma, error) {
	sma := &Sma{}

	if err := config.GetEnv(sma); err != nil {
		return nil, err
	}

	sma.logger = logger
	sma.realtime = realtime
	sma.private = private

	return sma, nil
}

func (m *Sma) Name() string {
	return "sma"
}

func (m *Sma) Execute(args []string) error {
	m.logger.Printf("start %s, version: %s, RiskRate: %f, Rot: %f, Interval: %v", m.Name(), config.Version, m.RiskRate, m.RotSize, m.Interval)

	for {
		ctx := context.Background()
		if err := m.executeCtx(ctx); err != nil {
			m.logger.Println(err)
		}
	}
}

func (m *Sma) executeCtx(ctx context.Context) error {
	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		return m.realtime.Subscribe(ctx)
	})
	eg.Go(func() error {
		return m.run(ctx)
	})

	return eg.Wait()
}

func (m *Sma) run(ctx context.Context) error {
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

func (m *Sma) trade(ctx context.Context) error {
	pos, err := m.private.GetPositions()
	if err != nil {
		return err
	}

	size := positionSize(pos)

	ex := m.realtime.GetExecutions(time.Second * 2)
	m.logger.Print(len(ex))
	mid, _, _ := m.realtime.GetBoard()

	v, err := sma(ex)
	if err != nil {
		return err
	}

	risk := m.RiskRate
	d2 := variance(ex)
	d := math.Pow(d2, 0.6)

	spread := risk * d
	if v > 0 {
		size -= 0.02
	} else if v < 0 {
		size += 0.02
	}
	offset := -spread * (size / m.RotSize) / m.MaxRot

	eg := errgroup.Group{}

	eg.Go(func() error {
		price := math.Floor(mid + offset + spread/2)
		id, err := m.private.CreateOrder("SELL", price, m.RotSize, "LIMIT")
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 8)
		m.logger.Printf("order created. sellID: %s, spread: %f, mid: %f, offset: %f, v: %f price: %f", id, spread, mid, offset, v, price)
		return m.private.CancelOrder(id)
	})
	eg.Go(func() error {
		price := math.Floor(mid + offset - spread/2)
		id, err := m.private.CreateOrder("BUY", price, m.RotSize, "LIMIT")
		if err != nil {
			return err
		}
		time.Sleep(time.Second * 8)
		m.logger.Printf("order created. buyID: %s, spread: %f, mid: %f, offset: %f, v: %f, price: %f", id, spread, mid, offset, v, price)
		return m.private.CancelOrder(id)
	})

	if err := eg.Wait(); err != nil {
		return err
	}

	return nil
}

func split(ex []*bitflyer.Execution, t time.Time) ([]*bitflyer.Execution, []*bitflyer.Execution, error) {
	before := []*bitflyer.Execution{}
	after := []*bitflyer.Execution{}

	for _, v := range ex {
		ts, err := parseTimeString(v.ExecDate)
		if err != nil {
			return nil, nil, err
		}
		if ts.Before(t) {
			before = append(before, v)
		} else {
			after = append(before, v)
		}
	}

	return before, after, nil
}

func average(ex []*bitflyer.Execution) float64 {
	sum := 0.0
	num := 0

	for _, v := range ex {
		sum += v.Price
		num++
	}

	return sum / float64(num)
}

func norm(ex []*bitflyer.Execution) []float64 {
	ave := average(ex)
	diff := []float64{}

	for _, v := range ex {
		diff = append(diff, v.Price-ave)
	}
	return diff
}

func sma(ex []*bitflyer.Execution) (float64, error) {
	before, after, err := split(ex, time.Now().Add(-time.Second))
	if err != nil {
		return 0.0, err
	}

	if len(before) < 1 || len(after) < 1 {
		return 0.0, nil
	}

	beforeNorm := norm(before)
	afterNorm := norm(after)

	sum := 0.0
	for _, v := range beforeNorm {
		sum += v
	}
	for _, v := range afterNorm {
		sum += v
	}
	ave := sum / float64(len(ex))

	return afterNorm[len(afterNorm)-1] - ave, nil
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

func parseTimeString(str string) (time.Time, error) {
	tmp, err := time.Parse(`2006-01-02T15:04:05.9999`, string(str[:len(str)-1]))
	jst := time.FixedZone("Asia/Tokyo", 9*60*60)
	return tmp.In(jst), err
}
