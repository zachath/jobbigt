package jobbigt

import (
	"bytes"
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

type AssertType string

// TODO: More types
const (
	// Only accepts intergers as value, anything else will result in an Error.
	StatusCode AssertType = "STATUS_CODE"

	// Only accepts BodyAssertion as value, anything else will result in an Error.
	Body AssertType = "BODY"
)

type BodyAssertion string

const (
	IsJson  BodyAssertion = "IS_JSON"
	IsEmpty BodyAssertion = "IS_EMPTY"
)

func isBodyAssertionvalue(v any) (BodyAssertion, bool) {
	if ba, ok := v.(BodyAssertion); ok {
		return ba, ok
	}

	return "", false
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
	timeout         int
	iterations      int
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

// Set request timeout. A request timing out will result in a result with the type Error.
func (r *Request) Timeout(timeout int) *Request {
	r.timeout = timeout
	return r
}

// Set request iterations, determines how many times the test is to be re-run if previous iteration exited with the result type of Reapeat.
// If exceeded the result type will be Error.
// Default 1
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

// Defines an assertion which acts as a simple test to ensure a certain value in the response is set correctly.
// These are run before the Test function if defined and run independently of it.
// Any failing assertions will fail the request.
func (r *Request) Assert(asertType AssertType, value any) *Request {
	r.assertions = append(r.assertions, func(response *http.Response) Result {
		switch asertType {
		case StatusCode:
			intValue, ok := value.(int)
			if !ok {
				return Result{
					Type:        Failure,
					Description: fmt.Sprintf("failed to cast value '%v' to type int", value),
				}
			} else if response.StatusCode != intValue {
				return Result{
					Type:        Failure,
					Description: fmt.Sprintf("received unexpected status code, exepcted %d but received %d", intValue, response.StatusCode),
				}
			}
		case Body:
			bodyAssertion, ok := isBodyAssertionvalue(value)
			if !ok {
				return Result{
					Type:        Failure,
					Description: fmt.Sprintf("%v is not a valid body assertion value", value),
				}
			}

			// TODO: Move to own funcs.
			switch bodyAssertion {
			case IsJson:
				body, err := io.ReadAll(response.Body)
				if err != nil {
					return Result{
						Type:        Failure,
						Description: "failed to read to response body",
					}
				}

				var js json.RawMessage
				err = json.Unmarshal(body, &js)
				if err != nil {
					return Result{
						Type:        Failure,
						Description: fmt.Sprintf("failed to unmarshal the response body: '%s'", body),
					}
				}
			case IsEmpty:
				b, err := io.ReadAll(response.Body)
				if err != nil {
					return Result{
						Type:        Failure, // TODO: Should these instead be Error?
						Description: "failed to read the response body",
					}
				} else if len(b) != 0 {
					return Result{
						Type:        Failure,
						Description: fmt.Sprintf("received non empty body, body had length of: %d", len(b)),
					}
				}
			default:
				return Result{
					Type:        Error,
					Description: fmt.Sprintf("'%s' is not a valid body assertion", bodyAssertion),
				}
			}
		default:
			return Result{
				Type:        Error,
				Description: fmt.Sprintf("'%s' is not a valid assert type", asertType),
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
