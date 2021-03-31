package runner

import (
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/mapsutil"
	"github.com/projectdiscovery/nuclei/v2/pkg/output"
	"github.com/projectdiscovery/nuclei/v2/pkg/templates"
	"github.com/remeh/sizedwaitgroup"
	"go.uber.org/atomic"
)

// processTemplateWithList process a template on the URL list
func (r *Runner) processTemplateWithList(template *templates.Template) bool {
	results := &atomic.Bool{}
	wg := sizedwaitgroup.New(r.options.BulkSize)
	r.hostMap.Scan(func(k, _ []byte) error {
		URL := string(k)

		wg.Add()
		go func(URL string) {
			defer wg.Done()

			match, err := template.Executer.Execute(URL)
			if err != nil {
				gologger.Warning().Msgf("[%s] Could not execute step: %s\n", r.colorizer.BrightBlue(template.ID), err)
			}
			results.CAS(false, match)
		}(URL)
		return nil
	})
	wg.Wait()
	return results.Load()
}

// processTemplateWithList process a template on the URL list
func (r *Runner) processWorkflowWithList(template *templates.Template) bool {
	results := &atomic.Bool{}
	wg := sizedwaitgroup.New(r.options.BulkSize)

	r.hostMap.Scan(func(k, _ []byte) error {
		URL := string(k)
		wg.Add()
		go func(URL string) {
			defer wg.Done()
			match := template.CompiledWorkflow.RunWorkflow(URL)
			results.CAS(false, match)
		}(URL)
		return nil
	})
	wg.Wait()
	return results.Load()
}

func (r *Runner) processTemplateWithResults(URL string, template *templates.Template) (map[interface{}]interface{}, error) {
	results := make(map[string]interface{})
	err := template.Executer.ExecuteWithResults(URL, func(result *output.InternalWrappedEvent) {
		results = mapsutil.MergeMaps(results, result.OperatorsResult.DynamicValues)
		results = mapsutil.MergeMaps(results, result.OperatorsResult.PayloadValues)
		for k, v := range result.OperatorsResult.Extracts {
			results[k] = v
		}
		for k, v := range result.OperatorsResult.Matches {
			results[k] = v
		}
		results["extracted"] = result.OperatorsResult.Extracted
		results["matched"] = result.OperatorsResult.Matched
		results["output_extracts"] = result.OperatorsResult.OutputExtracts

	})
	if err != nil {
		return nil, err
	}

	cresults := make(map[interface{}]interface{})
	for k, v := range results {
		cresults[k] = v
	}

	return cresults, nil
}
