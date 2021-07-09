package executer

import (
	"strings"

	"github.com/projectdiscovery/gologger"
	"github.com/yaklang/nuclei/v2/pkg/output"
	"github.com/yaklang/nuclei/v2/pkg/protocols"
)

// Executer executes a group of requests for a protocol
type Executer struct {
	requests []protocols.Request
	options  *protocols.ExecuterOptions
}

var _ protocols.Executer = &Executer{}

// NewExecuter creates a new request executer for list of requests
func NewExecuter(requests []protocols.Request, options *protocols.ExecuterOptions) *Executer {
	return &Executer{requests: requests, options: options}
}

// Compile compiles the execution generators preparing any requests possible.
func (e *Executer) Compile() error {
	for _, request := range e.requests {
		err := request.Compile(e.options)
		if err != nil {
			return err
		}
	}
	return nil
}

// Requests returns the total number of requests the rule will perform
func (e *Executer) Requests() int {
	var count int
	for _, request := range e.requests {
		count += request.Requests()
	}
	return count
}

// Execute executes the protocol group and returns true or false if results were found.
func (e *Executer) Execute(input string) (bool, error) {
	var results bool

	dynamicValues := make(map[string]interface{})
	previous := make(map[string]interface{})
	for _, req := range e.requests {
		req := req

		err := req.ExecuteWithResults(input, dynamicValues, previous, func(event *output.InternalWrappedEvent) {
			ID := req.GetID()
			if ID != "" {
				builder := &strings.Builder{}
				for k, v := range event.InternalEvent {
					builder.WriteString(ID)
					builder.WriteString("_")
					builder.WriteString(k)
					previous[builder.String()] = v
					builder.Reset()
				}
			}
			if event.OperatorsResult == nil {
				return
			}
			for _, result := range event.Results {
				if e.options.IssuesClient != nil {
					if err := e.options.IssuesClient.CreateIssue(result); err != nil {
						gologger.Warning().Msgf("Could not create issue on tracker: %s", err)
					}
				}
				results = true
				_ = e.options.Output.Write(result)
				e.options.Progress.IncrementMatched()
			}
		})
		if err != nil {
			gologger.Warning().Msgf("[%s] Could not execute request for %s: %s\n", e.options.TemplateID, input, err)
		}
	}
	return results, nil
}

// ExecuteWithResults executes the protocol requests and returns results instead of writing them.
func (e *Executer) ExecuteWithResults(input string, callback protocols.OutputEventCallback) error {
	dynamicValues := make(map[string]interface{})
	previous := make(map[string]interface{})

	for _, req := range e.requests {
		req := req

		err := req.ExecuteWithResults(input, dynamicValues, previous, func(event *output.InternalWrappedEvent) {
			ID := req.GetID()
			if ID != "" {
				builder := &strings.Builder{}
				for k, v := range event.InternalEvent {
					builder.WriteString(ID)
					builder.WriteString("_")
					builder.WriteString(k)
					previous[builder.String()] = v
					builder.Reset()
				}
			}
			if event.OperatorsResult == nil {
				return
			}
			callback(event)
		})
		if err != nil {
			gologger.Warning().Msgf("[%s] Could not execute request for %s: %s\n", e.options.TemplateID, input, err)
		}
	}
	return nil
}
