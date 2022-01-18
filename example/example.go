package main

import (
	"context"
	"encoding/json"
	"github.com/aidansteele/go-xrayprofile"
	"github.com/aws/aws-lambda-go/lambda"
	"time"
)

func main() {
	handler := lambda.NewHandler(handle)
	// nil options is also fine
	handler = xrayprofile.Wrap(handler, &xrayprofile.Options{})
	lambda.Start(handler)
}

func handle(ctx context.Context, input json.RawMessage) error {
	time.Sleep(2 * time.Second)
	return nil
}
