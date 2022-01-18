# `go-xrayprofile`

[AWS X-Ray][xray] is handy for understanding the overall performance of your
systems. Sometimes you want **much** greater function- or line-level detail
on what your Lambda function is busy doing - but not all the time, as profiling
can incur a performance impact. 

`go-xrayprofile` is a middleware that strikes a balance. It can selectively
profile (using [`fgprof`][fgprof]) the execution of a configurable percentage
of Lambda function invocations. These profiles are written to S3 for later
download and analysis using `go tool pprof`.

## Example usage

Integrate `go-xrayprofile` into your project by wrapping your existing Lambda
handler function like so:

```go
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
```

With the above options (the default), one in 65,536 invocations will be profiled.
To profile 1% of all invocations you can set `options.Threshold` to `655`, for
10% it would be `6553` and so on.

## How it works

The decision whether to profile or not is based on the last four hexadecimal
digits of the X-Ray root trace ID. E.g. if `options.Threshold` is set to `500`
and the trace header is `X-Amzn-Trace-Id: Root=1-5759e988-bd862e3fe1be46a99427[01F0]`
(brackets inserted for clarity) then the invocation **will** be profiled because
`0x01F0 <= 500`. 

We base this decision on the X-Ray trace header (rather than generating our own
random number) because then a single user-initiated trace can result in profiles
for every Lambda function associated with that trace stored together on S3. This
is likely more useful than unrelated traces for understanding end-to-end performance.

## Help wanted

It would be great if people took this idea and implemented it for other languages,
especially those that have native support on AWS Lambda. 

[xray]: https://aws.amazon.com/xray/
[fgprof]: https://github.com/felixge/fgprof