package tlsgateway

import (
	"math"
	"time"
)

// RetryConditionFunc determines whether a response should be retried.
type RetryConditionFunc func(resp *Response, err error) bool

// GetRetryIntervalFunc returns the interval before the next retry attempt.
type GetRetryIntervalFunc func(resp *Response, attempt int) time.Duration

func newBackoffInterval(min, max time.Duration) GetRetryIntervalFunc {
	return func(resp *Response, attempt int) time.Duration {
		d := time.Duration(math.Pow(2, float64(attempt))) * min
		if d > max {
			d = max
		}
		return d
	}
}

// RetryOnServerError retries on 5xx or transport errors.
func RetryOnServerError(resp *Response, err error) bool {
	return err != nil || (resp != nil && resp.StatusCode >= 500)
}

// RetryOnAnyError retries on any non-2xx response or error.
func RetryOnAnyError(resp *Response, err error) bool {
	return err != nil || (resp != nil && !resp.IsSuccess())
}
