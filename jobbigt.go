package jobbigt

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ResultType int

const (
	Success ResultType = iota
	Failure
	Stop
	Error
	Skip
	Repeat
	NoTest
)

type CleanUpResultType int

const (
	CleanUpSuccess CleanUpResultType = iota
	CleanUpFailure
)

type Result struct {
	Type           ResultType
	DownStreamArgs map[string]string
}

func (r *Result) Error() string {
	if r.DownStreamArgs != nil {
		return r.DownStreamArgs["error"]
	}
	return ""
}

type RequestGroup struct {
	id       string
	requests []*Request
}

func (rq *RequestGroup) Id(id string) *RequestGroup {
	rq.id = id
	return rq
}

func (rq *RequestGroup) Run() ResultType {
	for _, r := range rq.requests {
		results := r.Run()
		if results.Type == Skip {
			return Skip
		}
	}

	return Success
}

func (rq *RequestGroup) AddRequest(r *Request) {
	rq.requests = append(rq.requests, r)
}

type Request struct {
	id          string
	url         string
	method      string
	body        []byte
	timeout     int
	iterations  int
	testFunc    func(respone *http.Response, args ...any) Result
	cleanUpFunc func(testResult Result) CleanUpResultType
}

func newRequest(url, method string) *Request {
	return &Request{
		id:         uuid.NewString(),
		url:        url,
		method:     method,
		body:       nil,
		timeout:    100,
		iterations: 10,
	}
}

func Get(url string) *Request {
	return newRequest(url, http.MethodGet)
}

func Post(url string) *Request {
	return newRequest(url, http.MethodPost)
}

func (r *Request) Id(id string) *Request {
	r.id = id
	return r
}

func (r *Request) Body(body []byte) *Request {
	r.body = body
	return r
}

func (r *Request) Timeout(timeout int) *Request {
	r.timeout = timeout
	return r
}

func (r *Request) Iterations(iterations int) *Request {
	r.iterations = iterations
	return r
}

func (r *Request) perform() (*http.Response, error) {
	var reader io.Reader
	if r.body != nil {
		reader = bytes.NewReader(r.body)
	}

	request, err := http.NewRequest(r.method, r.url, reader)
	if err != nil {
		return nil, err
	}

	c := &http.Client{
		Timeout: time.Duration(r.timeout) * time.Second,
	}

	return c.Do(request)
}

func (r *Request) Run(args ...any) Result {
	response, err := r.perform()
	if err != nil {
		return Result{
			Type: Error,
			DownStreamArgs: map[string]string{
				"error": err.Error(),
			},
		}
	}

	result := Result{
		Type:           NoTest,
		DownStreamArgs: map[string]string{},
	}
	if r.testFunc != nil {
		result = r.testFunc(response, args...)
		if result.Type == Repeat {
			r.iterations--
			if r.iterations == 0 {
				return Result{
					Type: Failure,
				}
			}

			return r.Run(result.DownStreamArgs)
		}
	}

	if r.cleanUpFunc != nil {
		r.cleanUpFunc(result)
	}

	return result
}

func (r *Request) Test(testFunc func(response *http.Response, args ...any) Result) *Request {
	r.testFunc = testFunc
	return r
}

// TODO: Access to request?
func (r *Request) CleanUp(cleanUpFunc func(Result) CleanUpResultType) *Request {
	r.cleanUpFunc = cleanUpFunc
	return r
}
