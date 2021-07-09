package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"strings"

	"github.com/julienschmidt/httprouter"
	"github.com/yaklang/nuclei/v2/internal/testutils"
)

var httpTestcases = map[string]testutils.TestCase{
	"http/get-headers.yaml":           &httpGetHeaders{},
	"http/get-query-string.yaml":      &httpGetQueryString{},
	"http/get-redirects.yaml":         &httpGetRedirects{},
	"http/get.yaml":                   &httpGet{},
	"http/post-body.yaml":             &httpPostBody{},
	"http/post-json-body.yaml":        &httpPostJSONBody{},
	"http/post-multipart-body.yaml":   &httpPostMultipartBody{},
	"http/raw-cookie-reuse.yaml":      &httpRawCookieReuse{},
	"http/raw-dynamic-extractor.yaml": &httpRawDynamicExtractor{},
	"http/raw-get-query.yaml":         &httpRawGetQuery{},
	"http/raw-get.yaml":               &httpRawGet{},
	"http/raw-payload.yaml":           &httpRawPayload{},
	"http/raw-post-body.yaml":         &httpRawPostBody{},
	"http/raw-unsafe-request.yaml":    &httpRawUnsafeRequest{},
	"http/request-condition.yaml":     &httpRequestCondition{},
	"http/request-condition-new.yaml": &httpRequestCondition{},
}

func httpDebugRequestDump(r *http.Request) {
	if debug {
		if dump, err := httputil.DumpRequest(r, true); err == nil {
			fmt.Printf("\nRequest dump: \n%s\n\n", string(dump))
		}
	}
}

type httpGetHeaders struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpGetHeaders) Execute(filePath string) error {
	router := httprouter.New()
	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if strings.EqualFold(r.Header.Get("test"), "nuclei") {
			fmt.Fprintf(w, "This is test headers matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpGetQueryString struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpGetQueryString) Execute(filePath string) error {
	router := httprouter.New()
	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if strings.EqualFold(r.URL.Query().Get("test"), "nuclei") {
			fmt.Fprintf(w, "This is test querystring matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpGetRedirects struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpGetRedirects) Execute(filePath string) error {
	router := httprouter.New()
	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		http.Redirect(w, r, "/redirected", http.StatusFound)
	}))
	router.GET("/redirected", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		fmt.Fprintf(w, "This is test redirects matcher text")
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpGet struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpGet) Execute(filePath string) error {
	router := httprouter.New()
	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		fmt.Fprintf(w, "This is test matcher text")
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpPostBody struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpPostBody) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseForm(); err != nil {
			routerErr = err
			return
		}
		if strings.EqualFold(r.Form.Get("username"), "test") && strings.EqualFold(r.Form.Get("password"), "nuclei") {
			fmt.Fprintf(w, "This is test post-body matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpPostJSONBody struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpPostJSONBody) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)

		type doc struct {
			Username string `json:"username"`
			Password string `json:"password"`
		}
		obj := &doc{}
		if err := json.NewDecoder(r.Body).Decode(obj); err != nil {
			routerErr = err
			return
		}
		if strings.EqualFold(obj.Username, "test") && strings.EqualFold(obj.Password, "nuclei") {
			fmt.Fprintf(w, "This is test post-json-body matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpPostMultipartBody struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpPostMultipartBody) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseMultipartForm(1 * 1024); err != nil {
			routerErr = err
			return
		}
		password, ok := r.MultipartForm.Value["password"]
		if !ok || len(password) != 1 {
			routerErr = errors.New("no password in request")
			return
		}
		file := r.MultipartForm.File["username"]
		if len(file) != 1 {
			routerErr = errors.New("no file in request")
			return
		}
		if strings.EqualFold(password[0], "nuclei") && strings.EqualFold(file[0].Filename, "username") {
			fmt.Fprintf(w, "This is test post-multipart matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawDynamicExtractor struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawDynamicExtractor) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseForm(); err != nil {
			routerErr = err
			return
		}
		if strings.EqualFold(r.Form.Get("testing"), "parameter") {
			fmt.Fprintf(w, "Token: 'nuclei'")
		}
	}))
	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if strings.EqualFold(r.URL.Query().Get("username"), "nuclei") {
			fmt.Fprintf(w, "Test is test-dynamic-extractor-raw matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawGetQuery struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawGetQuery) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if strings.EqualFold(r.URL.Query().Get("test"), "nuclei") {
			fmt.Fprintf(w, "Test is test raw-get-query-matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawGet struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawGet) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)

		fmt.Fprintf(w, "Test is test raw-get-matcher text")
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawPayload struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawPayload) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseForm(); err != nil {
			routerErr = err
			return
		}
		if !(strings.EqualFold(r.Header.Get("another_header"), "bnVjbGVp") || strings.EqualFold(r.Header.Get("another_header"), "Z3Vlc3Q=")) {
			return
		}
		if strings.EqualFold(r.Form.Get("username"), "test") && (strings.EqualFold(r.Form.Get("password"), "nuclei") || strings.EqualFold(r.Form.Get("password"), "guest")) {
			fmt.Fprintf(w, "Test is raw-payload matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 2 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawPostBody struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawPostBody) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseForm(); err != nil {
			routerErr = err
			return
		}
		if strings.EqualFold(r.Form.Get("username"), "test") && strings.EqualFold(r.Form.Get("password"), "nuclei") {
			fmt.Fprintf(w, "Test is test raw-post-body-matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawCookieReuse struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawCookieReuse) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.POST("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseForm(); err != nil {
			routerErr = err
			return
		}
		if strings.EqualFold(r.Form.Get("testing"), "parameter") {
			http.SetCookie(w, &http.Cookie{Name: "nuclei", Value: "test"})
		}
	}))
	router.GET("/", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		if err := r.ParseForm(); err != nil {
			routerErr = err
			return
		}
		cookie, err := r.Cookie("nuclei")
		if err != nil {
			routerErr = err
			return
		}

		if strings.EqualFold(cookie.Value, "test") {
			fmt.Fprintf(w, "Test is test-cookie-reuse matcher text")
		}
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRawUnsafeRequest struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRawUnsafeRequest) Execute(filePath string) error {
	var routerErr error

	ts := testutils.NewTCPServer(func(conn net.Conn) {
		defer conn.Close()
		_, _ = conn.Write([]byte("HTTP/1.1 200 OK\r\nConnection: close\r\nContent-Length: 36\r\nContent-Type: text/plain; charset=utf-8\r\n\r\nThis is test raw-unsafe-matcher test"))
	})
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, "http://"+ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}

type httpRequestCondition struct{}

// Executes executes a test case and returns an error if occurred
func (h *httpRequestCondition) Execute(filePath string) error {
	router := httprouter.New()
	var routerErr error

	router.GET("/200", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		w.WriteHeader(200)
	}))
	router.GET("/400", httprouter.Handle(func(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
		httpDebugRequestDump(r)
		w.WriteHeader(400)
	}))
	ts := httptest.NewServer(router)
	defer ts.Close()

	results, err := testutils.RunNucleiAndGetResults(filePath, ts.URL, debug)
	if err != nil {
		return err
	}
	if routerErr != nil {
		return routerErr
	}
	if len(results) != 1 {
		return errIncorrectResultsCount(results)
	}
	return nil
}
