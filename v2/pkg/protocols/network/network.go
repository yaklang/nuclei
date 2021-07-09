package network

import (
	"net"
	"strings"

	"github.com/pkg/errors"
	"github.com/projectdiscovery/fastdialer/fastdialer"
	"github.com/yaklang/nuclei/v2/pkg/operators"
	"github.com/yaklang/nuclei/v2/pkg/protocols"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/expressions"
	"github.com/yaklang/nuclei/v2/pkg/protocols/network/networkclientpool"
)

// Request contains a Network protocol request to be made from a template
type Request struct {
	ID string `yaml:"id"`

	// Address is the address to send requests to (host:port:tls combos generally)
	Address   []string `yaml:"host"`
	addresses []addressKV

	// Payload is the payload to send for the network request
	Inputs []*Input `yaml:"inputs"`
	// ReadSize is the size of response to read (1024 if not provided by default)
	ReadSize int `yaml:"read-size"`

	// Operators for the current request go here.
	operators.Operators `yaml:",inline,omitempty"`
	CompiledOperators   *operators.Operators

	// cache any variables that may be needed for operation.
	dialer  *fastdialer.Dialer
	options *protocols.ExecuterOptions
}

type addressKV struct {
	ip   string
	port string
	tls  bool
}

// Input is the input to send on the network
type Input struct {
	// Data is the data to send as the input
	Data string `yaml:"data"`
	// Type is the type of input - hex, text.
	Type string `yaml:"type"`
	// Read is the number of bytes to read from socket
	Read int `yaml:"read"`
	// Name is the optional name of the input to provide matching on
	Name string `yaml:"name"`
}

// GetID returns the unique ID of the request if any.
func (r *Request) GetID() string {
	return r.ID
}

// Compile compiles the protocol request for further execution.
func (r *Request) Compile(options *protocols.ExecuterOptions) error {
	var shouldUseTLS bool
	var err error

	for _, address := range r.Address {
		// check if the connection should be encrypted
		if strings.HasPrefix(address, "tls://") {
			shouldUseTLS = true
			address = strings.TrimPrefix(address, "tls://")
		}
		if strings.Contains(address, ":") {
			addressHost, addressPort, portErr := net.SplitHostPort(address)
			if portErr != nil {
				return errors.Wrap(portErr, "could not parse address")
			}
			r.addresses = append(r.addresses, addressKV{ip: addressHost, port: addressPort, tls: shouldUseTLS})
		} else {
			r.addresses = append(r.addresses, addressKV{ip: address, tls: shouldUseTLS})
		}
	}
	// Pre-compile any input dsl functions before executing the request.
	for _, input := range r.Inputs {
		if input.Type != "" {
			continue
		}
		if compiled, evalErr := expressions.Evaluate(input.Data, map[string]interface{}{}); evalErr == nil {
			input.Data = compiled
		}
	}

	// Create a client for the class
	client, err := networkclientpool.Get(options.Options, &networkclientpool.Configuration{})
	if err != nil {
		return errors.Wrap(err, "could not get network client")
	}
	r.dialer = client

	if len(r.Matchers) > 0 || len(r.Extractors) > 0 {
		compiled := &r.Operators
		if err := compiled.Compile(); err != nil {
			return errors.Wrap(err, "could not compile operators")
		}
		r.CompiledOperators = compiled
	}
	r.options = options
	return nil
}

// Requests returns the total number of requests the YAML rule will perform
func (r *Request) Requests() int {
	return len(r.Address)
}
