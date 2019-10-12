package logger

import (
	"context"
	"log"

	"cloud.google.com/go/logging"
)

func NewLogger(projectID, name string) (*log.Logger, error) {
	client, err := logging.NewClient(context.Background(), projectID)
	if err != nil {
		return nil, err
	}

	return client.Logger(name).StandardLogger(logging.Info), nil
}
