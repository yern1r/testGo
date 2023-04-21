package main

import (
	"encoding/json"
	"expvar"
	"greenlight.bcc/internal/data"
	"net/http"
	"net/http/httptest"
	"testing"
)

const succeed = "\u2713"
const failed = "\u2717"

func TestRecoverPanic(t *testing.T) {

	app := newTestApplication(t)
	var tests = []struct {
		name           string
		method         func(w http.ResponseWriter, r *http.Request)
		wantCode       int
		expectedHeader string
	}{
		{
			"Panic occurred",
			func(w http.ResponseWriter, r *http.Request) { panic("This is panic situation") },
			http.StatusInternalServerError,
			"close",
		},
		{
			"Successfully ended session",
			func(w http.ResponseWriter, r *http.Request) {},
			http.StatusOK,
			"",
		},
	}

	for _, e := range tests {
		nextHandler := http.HandlerFunc(e.method)
		handlerToTest := app.recoverPanic(nextHandler)

		req := httptest.NewRequest("GET", "http://testing", nil)
		rr := httptest.NewRecorder()

		handlerToTest.ServeHTTP(rr, req)

		if rr.Header().Get("Connection") != e.expectedHeader {
			t.Errorf("TEST %s\t%s: expected header '%v', and get '%v' header value", failed, e.name, e.expectedHeader, rr.Header().Get("Connection"))
		} else {
			t.Logf("TEST %s\t%s: expected header '%v', and get '%v' header value", succeed, e.name, e.expectedHeader, rr.Header().Get("Connection"))
		}

		if rr.Result().StatusCode != e.wantCode {
			t.Errorf("TEST %s\t%s: expected status %v, and get %v status", failed, e.name, e.wantCode, rr.Result().StatusCode)
		} else {
			t.Logf("TEST %s\t%s: expected status %v, and get %v status", succeed, e.name, e.wantCode, rr.Result().StatusCode)
		}
	}
}

func TestRateLimit(t *testing.T) {
	app := newTestApplication(t)
	var tests = []struct {
		name     string
		addr     string
		reqNum   int
		wantCode int
	}{
		{
			"Valid request",
			"hello:world",
			1,
			http.StatusOK,
		},
		{
			"Empty network address",
			"",
			1,
			http.StatusInternalServerError,
		},
		{
			"Exceeded request limit",
			"hello:world",
			5,
			http.StatusTooManyRequests,
		},
	}

	for _, e := range tests {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
		handlerToTest := app.rateLimit(nextHandler)

		req := httptest.NewRequest("GET", "http://testing", nil)
		rr := httptest.NewRecorder()

		req.RemoteAddr = e.addr

		for i := 1; i <= e.reqNum; i++ {
			handlerToTest.ServeHTTP(rr, req)
		}

		if rr.Result().StatusCode != e.wantCode {
			t.Errorf("TEST %s\t%s: expected status %v, and get %v status", failed, e.name, e.wantCode, rr.Result().StatusCode)
		} else {
			t.Logf("TEST %s\t%s: expected status %v, and get %v status", succeed, e.name, e.wantCode, rr.Result().StatusCode)
		}
	}
}

func TestAuthenticate(t *testing.T) {
	app := newTestApplication(t)
	var tests = []struct {
		name                 string
		authHeader           string
		expectedContextEmail string
		wantCode             int
	}{
		{
			"Successfully return user",
			"Bearer bbbbbbbbbbbbbbbbbbbbbbbbbb",
			"example@gmail.com",
			http.StatusOK,
		},
		{
			"Unauthorized access 1",
			"Bearer aaaaaaaaaaaaaaaaaaaaaaaaaa",
			"",
			http.StatusUnauthorized,
		},
		{
			"Unauthorized access 2",
			"Bearer forifjd",
			"",
			http.StatusUnauthorized,
		},
		{
			"Unauthorized access 3",
			"forifjd",
			"",
			http.StatusUnauthorized,
		},
		{
			"Anonymous access",
			"",
			"",
			http.StatusOK,
		},
	}

	for _, e := range tests {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if app.contextGetUser(r).Email != e.expectedContextEmail {
				t.Errorf("TEST %s\t%s: expected '%v', and get '%v'", failed, e.name, e.expectedContextEmail, app.contextGetUser(r).Email)
			} else {
				t.Logf("TEST %s\t%s: expected '%v', and get '%v'", succeed, e.name, e.expectedContextEmail, app.contextGetUser(r).Email)
			}
		})
		handlerToTest := app.authenticate(nextHandler)

		req := httptest.NewRequest("GET", "http://testing", nil)
		rr := httptest.NewRecorder()
		req.Header.Add("Authorization", e.authHeader)

		handlerToTest.ServeHTTP(rr, req)

		if rr.Result().StatusCode != e.wantCode {
			t.Errorf("TEST %s\t%s: expected status %v, and get %v status", failed, e.name, e.wantCode, rr.Result().StatusCode)
		} else {
			t.Logf("TEST %s\t%s: expected status %v, and get %v status", succeed, e.name, e.wantCode, rr.Result().StatusCode)
		}
	}
}

func TestRequirePermission(t *testing.T) {
	app := newTestApplication(t)

	user := data.User{
		Name: "Amanzhol Bakhityar",
	}

	var tests = []struct {
		name       string
		id         int64
		activated  bool
		permission string
		wantCode   int
	}{
		{
			"Valid access",
			3,
			true,
			"movies:write",
			http.StatusOK,
		},
		{
			"Anonymous access",
			1,
			false,
			"movies:read",
			http.StatusUnauthorized,
		},
		{
			"Unactivated access",
			1,
			false,
			"movies:read",
			http.StatusForbidden,
		},
		{
			"Not privileged access",
			1,
			true,
			"movies:write",
			http.StatusForbidden,
		},
	}

	for _, e := range tests {
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

		handlerToTest := app.requirePermission(e.permission, nextHandler)

		req := httptest.NewRequest("GET", "http://testing", nil)
		rr := httptest.NewRecorder()

		if e.name == "Anonymous access" {
			req = app.contextSetUser(req, data.AnonymousUser)
		} else {
			user.ID = e.id
			user.Activated = e.activated
			req = app.contextSetUser(req, &user)
		}
		handlerToTest.ServeHTTP(rr, req)

		if rr.Result().StatusCode != e.wantCode {
			t.Errorf("TEST %s\t%s: expected status %v, and get %v status", failed, e.name, e.wantCode, rr.Result().StatusCode)
		} else {
			t.Logf("TEST %s\t%s: expected status %v, and get %v status", succeed, e.name, e.wantCode, rr.Result().StatusCode)
		}
	}
}

func TestEnableCORS(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	app := newTestApplication(t)

	var tests = []struct {
		name         string
		method       string
		origin       string
		headerVary   string
		headerMethod string
		headerAllow  string
		wantCode     int
	}{
		{
			"Restricted origin",
			"OPTIONS",
			"",
			"Origin",
			"",
			"",
			http.StatusOK,
		},
		{
			"Permitted origin",
			"OPTIONS",
			"localhost:8080",
			"Origin",
			"OPTIONS, PUT, PATCH, DELETE",
			"Authorization, Content-Type",
			http.StatusOK,
		},
	}

	for _, e := range tests {
		handlerToTest := app.enableCORS(nextHandler)

		req := httptest.NewRequest(e.method, "http://testing", nil)
		rr := httptest.NewRecorder()
		req.Header.Add("Origin", e.origin)
		req.Header.Add("Access-Control-Request-Method", e.method)
		handlerToTest.ServeHTTP(rr, req)

		if e.headerVary != rr.Header().Get("Vary") {
			t.Errorf("TEST %s\t%s: expected header '%v', but should be '%v'", failed, e.name, e.headerVary, rr.Header().Get("Vary"))
		} else {
			t.Logf("TEST %s\t%s: expected header '%v', and get '%v'", succeed, e.name, e.headerVary, rr.Header().Get("Vary"))
		}

		if rr.Header().Get("Access-Control-Allow-Origin") != e.origin {
			t.Errorf("TEST %s\t%s: origin header is '%v' but should be '%v'", failed, e.name, rr.Header().Get("Access-Control-Allow-Origin"), e.origin)
		} else {
			t.Logf("TEST %s\t%s: origin header is '%v'", succeed, e.name, rr.Header().Get("Access-Control-Allow-Origin"))
		}

		if rr.Header().Get("Access-Control-Allow-Methods") != e.headerMethod {
			t.Errorf("TEST %s\t%s: method header is '%v' but should be %v", failed, e.name, rr.Header().Get("Access-Control-Allow-Methods"), e.headerMethod)
		} else {
			t.Logf("TEST %s\t%s: method header is '%v'", succeed, e.name, rr.Header().Get("Access-Control-Allow-Methods"))
		}

		if rr.Header().Get("Access-Control-Allow-Headers") != e.headerAllow {
			t.Errorf("TEST %s\t%s: allowed headers are '%v' but should be %v", failed, e.name, rr.Header().Get("Access-Control-Allow-Headers"), e.headerAllow)
		} else {
			t.Logf("TEST %s\t%s: allowed headers are '%v'", succeed, e.name, rr.Header().Get("Access-Control-Allow-Headers"))
		}

		if rr.Result().StatusCode != e.wantCode {
			t.Errorf("TEST %s\t%s: expected status %v, and get %v status", failed, e.name, e.wantCode, rr.Result().StatusCode)
		} else {
			t.Logf("TEST %s\t%s: expected status %v, and get %v status", succeed, e.name, e.wantCode, rr.Result().StatusCode)
		}

	}
}

func TestMetrics(t *testing.T) {
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	app := newTestApplication(t)

	var tests = []struct {
		name       string
		expectedRR int
		expectedRS int
		wantCode   string
	}{
		{
			"Successful one request",
			1,
			1,
			"200",
		},
	}

	for _, e := range tests {
		handlerToTest := app.metrics(nextHandler)

		req := httptest.NewRequest("GET", "http://testing", nil)
		rr := httptest.NewRecorder()
		handlerToTest.ServeHTTP(rr, req)

		expvar.Handler().ServeHTTP(rr, req)

		var metrics struct {
			TotalRequestsReceived           int                 `json:"total_requests_received"`
			TotalResponsesSent              int                 `json:"total_responses_sent"`
			TotalProcessingTimeMicroseconds int                 `json:"total_processing_time_μs"`
			TotalResponsesSentByStatus      map[int]interface{} `json:"total_responses_sent_by_status"`
		}

		dec := json.NewDecoder(rr.Body)
		dec.DisallowUnknownFields()

		err := dec.Decode(&metrics)
		if err != nil {
			t.Log(err)
		}

		if metrics.TotalRequestsReceived != e.expectedRR {
			t.Errorf("TEST %s\t%s: received %v request but got %v", failed, e.name, e.expectedRR, metrics.TotalRequestsReceived)
		} else {
			t.Logf("TEST %s\t%s: as expected received %v request", succeed, e.name, e.expectedRR)
		}

		if metrics.TotalResponsesSent != e.expectedRS {
			t.Errorf("TEST %s\t%s: we sent %v response but got %v", failed, e.name, e.expectedRS, metrics.TotalResponsesSent)
		} else {
			t.Logf("TEST %s\t%s: as expected we sent %v response", succeed, e.name, e.expectedRS)
		}

		if metrics.TotalProcessingTimeMicroseconds == 0 {
			t.Errorf("TEST %s\t%s: request cannot be processed in 0 Microseconds", failed, e.name)
		} else {
			t.Logf("TEST %s\t%s: request was processed in  %v Microseconds", succeed, e.name, metrics.TotalProcessingTimeMicroseconds)
		}

		if int(metrics.TotalResponsesSentByStatus[200].(float64)) != 1 {
			t.Errorf("TEST %s\t%s: expected status %v", failed, e.name, e.wantCode)
		} else {
			t.Logf("TEST %s\t%s: expected status %v", succeed, e.name, e.wantCode)
		}
	}
}