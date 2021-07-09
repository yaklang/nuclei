package http

import (
	"context"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/corpix/uarand"
	"github.com/pkg/errors"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/expressions"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/generators"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/replacer"
	"github.com/yaklang/nuclei/v2/pkg/protocols/http/race"
	"github.com/yaklang/nuclei/v2/pkg/protocols/http/raw"
	"github.com/projectdiscovery/rawhttp"
	"github.com/projectdiscovery/retryablehttp-go"
)

var (
	urlWithPortRegex = regexp.MustCompile(`{{BaseURL}}:(\d+)`)
)

// generatedRequest is a single wrapped generated request for a template request
type generatedRequest struct {
	original        *Request
	rawRequest      *raw.Request
	meta            map[string]interface{}
	pipelinedClient *rawhttp.PipelineClient
	request         *retryablehttp.Request
}

// Make creates a http request for the provided input.
// It returns io.EOF as error when all the requests have been exhausted.
func (r *requestGenerator) Make(baseURL string, dynamicValues map[string]interface{}, interactURL string) (*generatedRequest, error) {
	// We get the next payload for the request.
	data, payloads, ok := r.nextValue()
	if !ok {
		return nil, io.EOF
	}
	ctx := context.Background()

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}

	data, parsed = baseURLWithTemplatePrefs(data, parsed)
	values := generators.MergeMaps(dynamicValues, map[string]interface{}{
		"Hostname": parsed.Host,
	})

	isRawRequest := len(r.request.Raw) > 0
	if !isRawRequest && strings.HasSuffix(parsed.Path, "/") && strings.Contains(data, "{{BaseURL}}/") {
		parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	}
	parsedString := parsed.String()
	values["BaseURL"] = parsedString

	// If data contains \n it's a raw request, process it like raw. Else
	// continue with the template based request flow.
	if isRawRequest {
		return r.makeHTTPRequestFromRaw(ctx, parsedString, data, values, payloads, interactURL)
	}
	return r.makeHTTPRequestFromModel(ctx, data, values, interactURL)
}

// Total returns the total number of requests for the generator
func (r *requestGenerator) Total() int {
	if r.payloadIterator != nil {
		return len(r.request.Raw) * r.payloadIterator.Remaining()
	}
	return len(r.request.Path)
}

// baseURLWithTemplatePrefs returns the url for BaseURL keeping
// the template port and path preference over the user provided one.
func baseURLWithTemplatePrefs(data string, parsed *url.URL) (string, *url.URL) {
	// template port preference over input URL port if template has a port
	matches := urlWithPortRegex.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		return data, parsed
	}
	port := matches[0][1]
	parsed.Host = net.JoinHostPort(parsed.Hostname(), port)
	data = strings.ReplaceAll(data, ":"+port, "")
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return data, parsed
}

// MakeHTTPRequestFromModel creates a *http.Request from a request template
func (r *requestGenerator) makeHTTPRequestFromModel(ctx context.Context, data string, values map[string]interface{}, interactURL string) (*generatedRequest, error) {
	final := replacer.Replace(data, values)
	if interactURL != "" {
		final = r.options.Interactsh.ReplaceMarkers(final, interactURL)
	}

	// Build a request on the specified URL
	req, err := http.NewRequestWithContext(ctx, r.request.Method, final, nil)
	if err != nil {
		return nil, err
	}

	request, err := r.fillRequest(req, values, interactURL)
	if err != nil {
		return nil, err
	}
	return &generatedRequest{request: request, original: r.request}, nil
}

// makeHTTPRequestFromRaw creates a *http.Request from a raw request
func (r *requestGenerator) makeHTTPRequestFromRaw(ctx context.Context, baseURL, data string, values, payloads map[string]interface{}, interactURL string) (*generatedRequest, error) {
	if interactURL != "" {
		data = r.options.Interactsh.ReplaceMarkers(data, interactURL)
	}
	return r.handleRawWithPayloads(ctx, data, baseURL, values, payloads)
}

// handleRawWithPayloads handles raw requests along with payloads
func (r *requestGenerator) handleRawWithPayloads(ctx context.Context, rawRequest, baseURL string, values, generatorValues map[string]interface{}) (*generatedRequest, error) {
	// Combine the template payloads along with base
	// request values.
	finalValues := generators.MergeMaps(generatorValues, values)

	// Evaulate the expressions for raw request if any.
	var err error
	rawRequest, err = expressions.Evaluate(rawRequest, finalValues)
	if err != nil {
		return nil, errors.Wrap(err, "could not evaluate helper expressions")
	}
	rawRequestData, err := raw.Parse(rawRequest, baseURL, r.request.Unsafe)
	if err != nil {
		return nil, err
	}

	// Unsafe option uses rawhttp library
	if r.request.Unsafe {
		unsafeReq := &generatedRequest{rawRequest: rawRequestData, meta: generatorValues, original: r.request}
		return unsafeReq, nil
	}

	// retryablehttp
	var body io.ReadCloser
	body = ioutil.NopCloser(strings.NewReader(rawRequestData.Data))
	if r.request.Race {
		// More or less this ensures that all requests hit the endpoint at the same approximated time
		// Todo: sync internally upon writing latest request byte
		body = race.NewOpenGateWithTimeout(body, time.Duration(2)*time.Second)
	}

	req, err := http.NewRequestWithContext(ctx, rawRequestData.Method, rawRequestData.FullURL, body)
	if err != nil {
		return nil, err
	}
	for key, value := range rawRequestData.Headers {
		if key == "" {
			continue
		}
		req.Header[key] = []string{value}
		if key == "Host" {
			req.Host = value
		}
	}
	request, err := r.fillRequest(req, values, "")
	if err != nil {
		return nil, err
	}

	return &generatedRequest{request: request, meta: generatorValues, original: r.request}, nil
}

// fillRequest fills various headers in the request with values
func (r *requestGenerator) fillRequest(req *http.Request, values map[string]interface{}, interactURL string) (*retryablehttp.Request, error) {
	// Set the header values requested
	for header, value := range r.request.Headers {
		if interactURL != "" {
			value = r.options.Interactsh.ReplaceMarkers(value, interactURL)
		}
		req.Header[header] = []string{replacer.Replace(value, values)}
		if header == "Host" {
			req.Host = replacer.Replace(value, values)
		}
	}

	// In case of multiple threads the underlying connection should remain open to allow reuse
	if r.request.Threads <= 0 && req.Header.Get("Connection") == "" {
		req.Close = true
	}

	// Check if the user requested a request body
	if r.request.Body != "" {
		body := r.request.Body
		if interactURL != "" {
			body = r.options.Interactsh.ReplaceMarkers(body, interactURL)
		}
		req.Body = ioutil.NopCloser(strings.NewReader(body))
	}
	setHeader(req, "User-Agent", uarand.GetRandom())

	// Only set these headers on non raw requests
	if len(r.request.Raw) == 0 {
		setHeader(req, "Accept", "*/*")
		setHeader(req, "Accept-Language", "en")
	}
	return retryablehttp.FromRequest(req)
}

// setHeader sets some headers only if the header wasn't supplied by the user
func setHeader(req *http.Request, name, value string) {
	if _, ok := req.Header[name]; !ok {
		req.Header.Set(name, value)
	}
	if name == "Host" {
		req.Host = value
	}
}
