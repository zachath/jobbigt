package jobbigt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type ResultType int

// Describes the result of a test.
const (
	Success ResultType = iota
	Failure
	Stop
	Error
	Skip
	Repeat
	NoTest
)

type PostRequestResultType int

const (
	PostRequestSuccess PostRequestResultType = iota
	PostRequestFailure
)

type Result struct {
	Type           ResultType
	Description    string
	DownStreamArgs map[string]string
}

func (r *Result) Error() string {
	if r.Type == Error {
		return r.Description
	}
	return ""
}

// TODO: The result on a request basis needs to be handled.
type RequestGroup struct {
	id       string
	requests []*Request
}

func (rq *RequestGroup) Id(id string) *RequestGroup {
	rq.id = id
	return rq
}

func (rq *RequestGroup) Run() *Result {
	for _, r := range rq.requests {
		result := r.Run()
		if result.Type == Skip {
			return &Result{
				Type:        Skip,
				Description: fmt.Sprintf("Skipped caused by request %s", r.id),
			}
		}
	}

	return &Result{
		Type: Success,
	}
}

func (rq *RequestGroup) AddRequest(r *Request) {
	rq.requests = append(rq.requests, r)
}

type Request struct {
	id              string
	url             string
	method          string
	body            []byte
	headers         http.Header
	sleep           time.Duration
	timeout         int
	iterations      int
	responseBody    []byte
	preRequestFunc  func() //TODO
	testFunc        func(respone *http.Response, args ...any) Result
	postRequestFunc func(testResult Result) PostRequestResultType
	assertions      []func(response *http.Response) Result
}

func newRequest(url, method string) *Request {
	return &Request{
		id:         uuid.NewString(),
		url:        url,
		method:     method,
		body:       nil,
		headers:    http.Header{},
		timeout:    100,
		iterations: 1,
	}
}

// Creates a new GET request.
func Get(url string) *Request {
	return newRequest(url, http.MethodGet)
}

// Creates a new POST request.
func Post(url string) *Request {
	return newRequest(url, http.MethodPost)
}

// Set request id.
func (r *Request) Id(id string) *Request {
	r.id = id
	return r
}

// Set request body.
func (r *Request) Body(body []byte) *Request {
	r.body = body
	return r
}

// Set request header key value pair.
func (r *Request) Header(key, value string) *Request {
	r.headers.Add(key, value)
	return r
}

func (r *Request) BasicAuth(username, password string) *Request {
	r.headers.Add("Authorization", fmt.Sprintf("Basic %s", base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", username, password)))))
	return r
}

// Set the duration to sleep between iterations.
// Default no sleep.
func (r *Request) Sleep(sleep time.Duration) *Request {
	r.sleep = sleep
	return r
}

// Set request timeout. A request timing out will result in a result with the type Error.
func (r *Request) Timeout(timeout int) *Request {
	r.timeout = timeout
	return r
}

// Set request iterations, determines how many times the test is to be re-run if previous iteration exited with the result type of Reapeat.
// If exceeded the result type will be Error.
// Any value below 1 will be ignored and set to the default value of 1
func (r *Request) Iterations(iterations int) *Request {
	if iterations >= 1 {
		r.iterations = iterations
	}
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
	request.Header = r.headers

	c := &http.Client{
		Timeout: time.Duration(r.timeout) * time.Second,
	}

	return c.Do(request)
}

func (r *Request) readBody(response *http.Response) error {
	b, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	r.responseBody = b

	return nil
}

// Performs the request, any pre-request/post-request functions, the test and assertions.
func (r *Request) Run(args ...any) *Result {
	if r.url == "" {
		return &Result{
			Type:        Error,
			Description: "url is required",
		}
	} else if r.method == "" {
		return &Result{
			Type:        Error,
			Description: "method is required",
		}
	}

	if r.preRequestFunc != nil {
		r.preRequestFunc()
	}

	response, err := r.perform()
	if err != nil {
		return &Result{
			Type:        Error,
			Description: err.Error(),
		}
	}

	err = r.readBody(response)
	if err != nil {
		return &Result{
			Type:        Error,
			Description: err.Error(),
		}
	}

	result := Result{
		Type:           Success,
		DownStreamArgs: map[string]string{},
	}
	if len(r.assertions) == 0 {
		result = Result{
			Type:           NoTest,
			DownStreamArgs: map[string]string{},
		}
	}

	assert := r.checkAssertions(response)
	if assert.Type != Success {
		return &assert
	}

	if r.testFunc != nil {
		result = r.testFunc(response, args...)
		if result.Type == Repeat {
			r.iterations--
			if r.iterations == 0 {
				return &Result{
					Type:        Failure,
					Description: "failed after running out of iterations",
				}
			}

			time.Sleep(r.sleep)

			return r.Run(result.DownStreamArgs)
		}
	}

	if r.postRequestFunc != nil {
		r.postRequestFunc(result) //TODO: Handle post up result
	}

	return &result
}

// Set the test function.
func (r *Request) Test(testFunc func(response *http.Response, args ...any) Result) *Request {
	r.testFunc = testFunc
	return r
}

// Set the pre-request function
func (r *Request) PreRequest(preRequestFunc func()) *Request {
	r.preRequestFunc = preRequestFunc
	return r
}

// TODO: Access to request?
// Set the post-request function
func (r *Request) PostRequest(postRequestFunc func(Result) PostRequestResultType) *Request {
	r.postRequestFunc = postRequestFunc
	return r
}

// Assert that the status code of the response is of a certain value. A mismatch in recived and expected results in a 'Failure'.
func (r *Request) StatusCode(expectedStatusCode int) *Request {
	r.assertions = append(r.assertions, func(response *http.Response) Result {
		if response.StatusCode != expectedStatusCode {
			return Result{
				Type:        Failure,
				Description: fmt.Sprintf("received unexpected status code, exepcted %d but received %d", expectedStatusCode, response.StatusCode),
			}
		}

		return Result{
			Type: Success,
		}
	})
	return r
}

// Assert that the response body is empty. A non empty response body results in a 'Failure'.
func (r *Request) BodyIsEmpty() *Request {
	r.assertions = append(r.assertions, func(response *http.Response) Result {
		if r.responseBody == nil {
			return Result{
				Type:        Failure,
				Description: "received nil response",
			}
		}

		if len(r.responseBody) != 0 {
			return Result{
				Type:        Failure,
				Description: fmt.Sprintf("received non empty body, body had length of: %d", len(r.responseBody)),
			}
		}

		return Result{
			Type: Success,
		}
	})
	return r
}

// Assert that the response body is json. A non json response body results in a 'Failure'.
func (r *Request) BodyIsJson() *Request {
	r.assertions = append(r.assertions, func(response *http.Response) Result {
		if r.responseBody == nil {
			return Result{
				Type:        Failure,
				Description: "received nil response",
			}
		}

		var js json.RawMessage
		err := json.Unmarshal(r.responseBody, &js)
		if err != nil {
			return Result{
				Type:        Failure,
				Description: fmt.Sprintf("failed to unmarshal the response body: '%s'", r.responseBody),
			}
		}

		return Result{
			Type: Success,
		}
	})
	return r
}

func (r *Request) checkAssertions(response *http.Response) Result {
	for _, assertion := range r.assertions {
		result := assertion(response)
		if result.Type != Success {
			return result
		}
	}
	return Result{
		Type: Success,
	}
}
