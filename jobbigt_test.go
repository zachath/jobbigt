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

	testResult := NewRequest(testServer.URL).
		Test(func(respone *http.Response, args ...any) Result {
			if respone.StatusCode == http.StatusOK {
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

func TestBasicCleanUp(t *testing.T) {
	var toggle bool
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		toggle = !toggle
		w.WriteHeader(http.StatusOK)
	}))

	testResult := NewRequest(testServer.URL).
		Test(func(respone *http.Response, args ...any) Result {
			if respone.StatusCode == http.StatusOK {
				return Result{
					Type: Success,
				}
			}
			return Result{
				Type: Failure,
			}
		}).
		CleanUp(func(r Result) CleanUpResultType {
			response, err := http.Get(testServer.URL)
			if err != nil || response.StatusCode != http.StatusOK {
				return CleanUpFailure
			}
			return CleanUpSuccess
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
