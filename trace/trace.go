package trace

import (
	"context"
	"fmt"
	"log"
	"time"

	monitoring "cloud.google.com/go/monitoring/apiv3"
	"github.com/golang/protobuf/ptypes/timestamp"
	"github.com/sodefrin/bitflyer"
	metricpb "google.golang.org/genproto/googleapis/api/metric"
	monitoringpb "google.golang.org/genproto/googleapis/monitoring/v3"
)

func Trace(ctx context.Context, logger *log.Logger, private *bitflyer.PrivateAPIClient, projectID string) error {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			c, err := private.GetCollateral()
			if err != nil {
				logger.Printf("failed to GetCollateral: %s", err.Error())
			}
			if err := traceCollateral(ctx, projectID, c.Collateral); err != nil {
				logger.Printf("failed to GetCollateral: %s", err.Error())
			}
			logger.Printf("trace collateral %f", c.Collateral)
		}
	}

	return nil
}

func traceCollateral(ctx context.Context, projectID string, value float64) error {
	c, err := monitoring.NewMetricClient(ctx)
	if err != nil {
		return err
	}
	now := &timestamp.Timestamp{
		Seconds: time.Now().Unix(),
	}
	req := &monitoringpb.CreateTimeSeriesRequest{
		Name: "projects/" + projectID,
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

	err = c.CreateTimeSeries(ctx, req)
	if err != nil {
		return fmt.Errorf("could not write time series value, %v ", err)
	}
	return nil
}
