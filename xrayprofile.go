package xrayprofile

import (
	"bytes"
	"context"
	"fmt"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-lambda-go/lambdacontext"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"github.com/felixge/fgprof"
	"os"
	"strconv"
	"strings"
)

type Options struct {
	S3        s3iface.S3API
	Threshold uint16
	Prefix    string
	Bucket    string
}

func Wrap(inner lambda.Handler, opts *Options) lambda.Handler {
	o := Options{}
	if opts != nil {
		o = *opts
	}

	api := o.S3
	if api == nil {
		sess, _ := session.NewSessionWithOptions(session.Options{SharedConfigState: session.SharedConfigEnable})
		api = s3.New(sess)
	}

	threshold := o.Threshold
	if threshold == 0 {
		u64, _ := strconv.ParseUint(os.Getenv("XRAYPROFILE_THRESHOLD"), 10, 16)
		threshold = uint16(u64)
	}

	prefix := o.Prefix
	if prefix == "" {
		prefix = os.Getenv("XRAYPROFILE_S3_PREFIX")
	}

	bucket := o.Bucket
	if bucket == "" {
		bucket = os.Getenv("XRAYPROFILE_S3_BUCKET")
	}

	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}

	return &handler{
		inner:     inner,
		buf:       &bytes.Buffer{},
		s3:        s3manager.NewUploaderWithClient(api),
		threshold: o.Threshold,
		prefix:    prefix,
		bucket:    o.Bucket,
	}
}

type handler struct {
	inner     lambda.Handler
	buf       *bytes.Buffer
	s3        *s3manager.Uploader
	threshold uint16
	prefix    string
	bucket    string
}

func (h *handler) Invoke(ctx context.Context, payload []byte) ([]byte, error) {
	lctx, _ := lambdacontext.FromContext(ctx)
	requestId := lctx.AwsRequestID
	th := parseTraceHeader(os.Getenv("_X_AMZN_TRACE_ID"))

	suffix := th.Root[len(th.Root)-4:]
	u64, _ := strconv.ParseUint(suffix, 16, 16)
	numeric := uint16(u64)

	if numeric <= h.threshold {
		h.buf.Reset()
		stop := fgprof.Start(h.buf, fgprof.FormatPprof)
		defer h.upload(ctx, th.Root, requestId, stop)
	}

	return h.inner.Invoke(ctx, payload)
}

func (h *handler) upload(ctx context.Context, traceId, requestId string, stop func() error) {
	err := stop()
	if err != nil {
		fmt.Fprintf(os.Stderr, "xrayprofile: not saving profile: %+v\n", err)
		return
	}

	if h.bucket == "" {
		// we have to wait until first invocation to introspect our aws account id.
		// right now we _could_ determine it from the access key id[1] but that feels
		// hacky and would require another dependency
		// [1]: https://awsteele.com/blog/2020/09/26/aws-access-key-format.html
		lctx, _ := lambdacontext.FromContext(ctx)
		accountId := strings.Split(lctx.InvokedFunctionArn, ":")[4]
		h.bucket = fmt.Sprintf("xrayprofile-%s-%s", accountId, os.Getenv("AWS_REGION"))
	}

	_, err = h.s3.UploadWithContext(ctx, &s3manager.UploadInput{
		Bucket: &h.bucket,
		Key:    aws.String(fmt.Sprintf("%s%s/%s.pprof", h.prefix, traceId, requestId)),
		Body:   h.buf,
		ACL:    aws.String(s3.ObjectCannedACLBucketOwnerFullControl),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "xrayprofile: failed to upload profile to s3: %+v\n", err)
		return
	}
}

type traceHeader struct {
	Root   string
	Parent string
}

func parseTraceHeader(input string) traceHeader {
	pairs := strings.Split(input, ";")
	m := map[string]string{}
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		m[kv[0]] = kv[1]
	}

	th := traceHeader{}
	th.Root = m["Root"]
	th.Parent = m["Parent"]
	return th
}
