package jobbigt

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

func isFailure(result *Result, expectedFailure bool) bool {
	return (result.Type == Success && expectedFailure) ||
		(result.Type != Success && !expectedFailure) ||
		(result.Type != Failure && expectedFailure) ||
		(result.Type == Failure && !expectedFailure)
}

func TestBasicStatusCode(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	testResult := Get(testServer.URL).
		Test(func(response *http.Response, args ...any) Result {
			if response.StatusCode == http.StatusOK {
				return Result{
					Type: Success,
				}
			}
			return Result{
				Type: Failure,
			}
		}).
		Run()

	if testResult.Type != Success {
		t.Errorf("received unexpected result type, expected '%d', got '%d'", Success, testResult.Type)
	}
}

func TestPreRequestFunc(t *testing.T) {
	for _, tc := range []struct {
		ExpectedFailure bool
	}{
		{
			ExpectedFailure: false,
		},
		{
			ExpectedFailure: true,
		},
	} {
		var toggle bool
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			toggle = !toggle
			w.WriteHeader(http.StatusOK)
		}))

		r := Get(testServer.URL).
			PreRequest(func() *Result {
				if tc.ExpectedFailure {
					return &Result{
						Type:        Failure,
						Description: "[ERROR]",
					}
				}

				_, err := http.Get(testServer.URL)
				if err != nil {
					return &Result{
						Type: Failure,
					}
				}

				return &Result{
					Type: Success,
				}
			}).
			Test(func(response *http.Response, args ...any) Result {
				if tc.ExpectedFailure {
					t.Fatal("Test fucntion ran even after pre request function failed")
				}
				return Result{
					Type: Success,
				}
			}).
			Run()

		if toggle {
			t.Error("expected toggle to be false")
		}

		if tc.ExpectedFailure {
			if r.Description != "received non successful result from pre request func: [ERROR]" {
				t.Errorf("received unexpected result description: '%s'", r.Description)
			}
		}
	}
}

func TestPostRequestFunc(t *testing.T) {

	for _, tc := range []struct {
		ExpectedFailure bool
	}{
		{
			ExpectedFailure: false,
		},
		{
			ExpectedFailure: true,
		},
	} {
		var toggle bool
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			toggle = !toggle
			w.WriteHeader(http.StatusOK)
		}))

		r := Get(testServer.URL).
			Test(func(response *http.Response, args ...any) Result {
				return Result{
					Type:        Success,
					Description: "The Test Result",
				}
			}).
			PostRequest(func(r *Result) *Result {
				if r.Type != Success || r.Description != "The Test Result" {
					t.Fatal("result provided is not the test result")
				}

				if tc.ExpectedFailure {
					return &Result{
						Type:        Failure,
						Description: "[ERROR]",
					}
				}

				_, err := http.Get(testServer.URL)
				if err != nil {
					return &Result{
						Type: Failure,
					}
				}

				return &Result{
					Type: Success,
				}
			}).
			Run()

		if tc.ExpectedFailure {
			if !toggle {
				t.Error("expected toggle to be true")
			}

			if r.Description != "received non successful result from post request func: [ERROR]" {
				t.Errorf("received unexpected result description: '%s'", r.Description)
			}
		} else {
			if toggle {
				t.Error("expected toggle to be false")
			}
		}
	}
}

func TestIteration(t *testing.T) {
	requiredAttempts := 5

	for id, tc := range []struct {
		Iterations      int
		ExpectedFailure bool
	}{
		{
			Iterations:      requiredAttempts,
			ExpectedFailure: false,
		},
		{
			Iterations:      requiredAttempts - 1,
			ExpectedFailure: true,
		},
	} {
		var attempts int
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			attempts++
			if attempts == requiredAttempts {
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusBadRequest)
			}
		}))

		result := Get(testServer.URL).Iterations(tc.Iterations).Test(func(response *http.Response, args ...any) Result {
			if response.StatusCode != http.StatusOK {
				return Result{
					Type: Repeat,
				}
			}

			return Result{
				Type: Success,
			}
		}).Run()

		if isFailure(result, tc.ExpectedFailure) {
			t.Errorf("(%d) %v", id, *result)
		}
	}
}

func TestUrlRequried(t *testing.T) {
	request := Request{method: http.MethodGet}
	result := request.Run()

	if result.Type != Error || result.Description != "url is required" {
		t.Errorf("expected error")
	}
}

func TestMethodRequried(t *testing.T) {
	request := Request{url: "url"}
	result := request.Run()

	if result.Type != Error || result.Description != "method is required" {
		t.Errorf("expected error")
	}
}

func TestStatusCodeAssertion(t *testing.T) {
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for id, tc := range []struct {
		Code            int
		ExpectedFailure bool
	}{
		{
			Code:            http.StatusOK,
			ExpectedFailure: false,
		},
		{
			Code:            http.StatusBadRequest,
			ExpectedFailure: true,
		},
	} {
		result := Get(testServer.URL).
			StatusCode(tc.Code).
			Run()

		if isFailure(result, tc.ExpectedFailure) {
			t.Errorf("(%d) %v", id, *result)
		}
	}
}

func TestBodyIsEmptyAssertion(t *testing.T) {
	for id, tc := range []struct {
		ExpectedFailure bool
	}{
		{
			ExpectedFailure: false,
		},
		{
			ExpectedFailure: true,
		},
	} {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.ExpectedFailure {
				w.Write([]byte("Some non empty response"))
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))

		result := Get(testServer.URL).
			BodyIsEmpty().
			Run()

		if isFailure(result, tc.ExpectedFailure) {
			t.Errorf("(%d) %v", id, *result)
		}
	}
}

func TestBodyIsJsonAssertion(t *testing.T) {
	for id, tc := range []struct {
		ExpectedFailure bool
	}{
		{
			ExpectedFailure: false,
		},
		{
			ExpectedFailure: true,
		},
	} {
		testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.ExpectedFailure {
				w.Write([]byte(`Non json response`))
			} else {
				w.Write([]byte(`{"key": "value"}`))
			}
		}))

		result := Get(testServer.URL).
			BodyIsJson().
			Run()

		if isFailure(result, tc.ExpectedFailure) {
			t.Errorf("(%d) %v", id, *result)
		}
	}
}

func TestHeader(t *testing.T) {
	headerKey := "Key"
	headerValue := "value"

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		arr, ok := r.Header[headerKey]
		if !ok {
			t.Error("did not received header key in request")
		}

		if !slices.Contains(arr, headerValue) {
			t.Error("did not received header value in request")
		}
	}))

	Get(testServer.URL).
		Header(headerKey, headerValue).
		Run()
}

func TestBasicAuth(t *testing.T) {
	username := "user"
	password := "password"

	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		if !ok || u != username || p != password {
			t.Errorf("did not recive expected basic auth: %s, %s, %t", u, p, ok)
		}
	}))

	Get(testServer.URL).
		BasicAuth(username, password).
		Run()
}
