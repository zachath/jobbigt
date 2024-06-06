package jobbigt

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

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
		fmt.Println(testResult.Error())
	}
}

func TestBasicPostRequest(t *testing.T) {
	var toggle bool
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toggle = !toggle
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
		PostRequest(func(r Result) PostRequestResultType {
			response, err := http.Get(testServer.URL)
			if err != nil || response.StatusCode != http.StatusOK {
				return PostRequestFailure
			}
			return PostRequestSuccess
		}).
		Run()

	if testResult.Type != Success {
		t.Errorf("received unexpected result type, expected '%d', got '%d'", Success, testResult.Type)
		fmt.Println(testResult.Error())
	}

	if toggle {
		t.Error("expected toggle to be false")
	}
}

func TestIteration(t *testing.T) {
	requiredAttempts := 5

	for id, tc := range []struct {
		Iterations    int
		ExpectedError bool
	}{
		{
			Iterations:    requiredAttempts,
			ExpectedError: false,
		},
		{
			Iterations:    requiredAttempts - 1,
			ExpectedError: true,
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

		if (result.Type == Success && tc.ExpectedError) || (result.Type != Failure && tc.ExpectedError) {
			t.Errorf("(%d) %v", id, result)
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
