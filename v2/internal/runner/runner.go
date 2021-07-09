package runner

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/logrusorgru/aurora"
	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/hmap/store/hybrid"
	"github.com/yaklang/nuclei/v2/internal/colorizer"
	"github.com/yaklang/nuclei/v2/pkg/catalog"
	"github.com/yaklang/nuclei/v2/pkg/output"
	"github.com/yaklang/nuclei/v2/pkg/progress"
	"github.com/yaklang/nuclei/v2/pkg/projectfile"
	"github.com/yaklang/nuclei/v2/pkg/protocols"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/clusterer"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/interactsh"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/protocolinit"
	"github.com/yaklang/nuclei/v2/pkg/protocols/headless/engine"
	"github.com/yaklang/nuclei/v2/pkg/reporting"
	"github.com/yaklang/nuclei/v2/pkg/reporting/exporters/disk"
	"github.com/yaklang/nuclei/v2/pkg/reporting/exporters/sarif"
	"github.com/yaklang/nuclei/v2/pkg/templates"
	"github.com/yaklang/nuclei/v2/pkg/types"
	"github.com/remeh/sizedwaitgroup"
	"github.com/rs/xid"
	"go.uber.org/atomic"
	"go.uber.org/ratelimit"
	"gopkg.in/yaml.v2"
)

// Runner is a client for running the enumeration process.
type Runner struct {
	hostMap         *hybrid.HybridMap
	output          output.Writer
	interactsh      *interactsh.Client
	inputCount      int64
	templatesConfig *nucleiConfig
	options         *types.Options
	projectFile     *projectfile.ProjectFile
	catalog         *catalog.Catalog
	progress        progress.Progress
	colorizer       aurora.Aurora
	issuesClient    *reporting.Client
	severityColors  *colorizer.Colorizer
	browser         *engine.Browser
	ratelimiter     ratelimit.Limiter
}

// New creates a new client for running enumeration process.
func New(options *types.Options) (*Runner, error) {
	runner := &Runner{
		options: options,
	}
	if options.Headless {
		browser, err := engine.New(options)
		if err != nil {
			return nil, err
		}
		runner.browser = browser
	}
	if err := runner.updateTemplates(); err != nil {
		gologger.Warning().Msgf("Could not update templates: %s\n", err)
	}

	runner.catalog = catalog.New(runner.options.TemplatesDirectory)
	// Read nucleiignore file if given a templateconfig
	if runner.templatesConfig != nil {
		runner.readNucleiIgnoreFile()
		runner.catalog.AppendIgnore(runner.templatesConfig.IgnorePaths)
	}
	var reportingOptions *reporting.Options
	if options.ReportingConfig != "" {
		file, err := os.Open(options.ReportingConfig)
		if err != nil {
			gologger.Fatal().Msgf("Could not open reporting config file: %s\n", err)
		}

		reportingOptions = &reporting.Options{}
		if parseErr := yaml.NewDecoder(file).Decode(reportingOptions); parseErr != nil {
			file.Close()
			gologger.Fatal().Msgf("Could not parse reporting config file: %s\n", parseErr)
		}
		file.Close()
	}
	if options.DiskExportDirectory != "" {
		if reportingOptions != nil {
			reportingOptions.DiskExporter = &disk.Options{Directory: options.DiskExportDirectory}
		} else {
			reportingOptions = &reporting.Options{}
			reportingOptions.DiskExporter = &disk.Options{Directory: options.DiskExportDirectory}
		}
	}
	if options.SarifExport != "" {
		if reportingOptions != nil {
			reportingOptions.SarifExporter = &sarif.Options{File: options.SarifExport}
		} else {
			reportingOptions = &reporting.Options{}
			reportingOptions.SarifExporter = &sarif.Options{File: options.SarifExport}
		}
	}
	if reportingOptions != nil {
		if client, err := reporting.New(reportingOptions, options.ReportingDB); err != nil {
			gologger.Fatal().Msgf("Could not create issue reporting client: %s\n", err)
		} else {
			runner.issuesClient = client
		}
	}

	// output coloring
	useColor := !options.NoColor
	runner.colorizer = aurora.NewAurora(useColor)
	runner.severityColors = colorizer.New(runner.colorizer)

	if options.TemplateList {
		runner.listAvailableTemplates()
		os.Exit(0)
	}

	if (len(options.Templates) == 0 || !options.NewTemplates || (options.Targets == "" && !options.Stdin && options.Target == "")) && options.UpdateTemplates {
		os.Exit(0)
	}
	if hm, err := hybrid.New(hybrid.DefaultDiskOptions); err != nil {
		gologger.Fatal().Msgf("Could not create temporary input file: %s\n", err)
	} else {
		runner.hostMap = hm
	}

	runner.inputCount = 0
	dupeCount := 0

	// Handle single target
	if options.Target != "" {
		runner.inputCount++
		// nolint:errcheck // ignoring error
		runner.hostMap.Set(options.Target, nil)
	}

	// Handle stdin
	if options.Stdin {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			url := strings.TrimSpace(scanner.Text())
			if url == "" {
				continue
			}
			if _, ok := runner.hostMap.Get(url); ok {
				dupeCount++
				continue
			}
			runner.inputCount++
			// nolint:errcheck // ignoring error
			runner.hostMap.Set(url, nil)
		}
	}

	// Handle taget file
	if options.Targets != "" {
		input, err := os.Open(options.Targets)
		if err != nil {
			gologger.Fatal().Msgf("Could not open targets file '%s': %s\n", options.Targets, err)
		}
		scanner := bufio.NewScanner(input)
		for scanner.Scan() {
			url := strings.TrimSpace(scanner.Text())
			if url == "" {
				continue
			}
			if _, ok := runner.hostMap.Get(url); ok {
				dupeCount++
				continue
			}
			runner.inputCount++
			// nolint:errcheck // ignoring error
			runner.hostMap.Set(url, nil)
		}
		input.Close()
	}

	if dupeCount > 0 {
		gologger.Info().Msgf("Supplied input was automatically deduplicated (%d removed).", dupeCount)
	}

	// Create the output file if asked
	outputWriter, err := output.NewStandardWriter(!options.NoColor, options.NoMeta, options.JSON, options.Output, options.TraceLogFile)
	if err != nil {
		gologger.Fatal().Msgf("Could not create output file '%s': %s\n", options.Output, err)
	}
	runner.output = outputWriter

	// Creates the progress tracking object
	var progressErr error
	runner.progress, progressErr = progress.NewStatsTicker(options.StatsInterval, options.EnableProgressBar, options.Metrics, options.MetricsPort)
	if progressErr != nil {
		return nil, progressErr
	}

	// create project file if requested or load existing one
	if options.Project {
		var projectFileErr error
		runner.projectFile, projectFileErr = projectfile.New(&projectfile.Options{Path: options.ProjectPath, Cleanup: options.ProjectPath == ""})
		if projectFileErr != nil {
			return nil, projectFileErr
		}
	}

	if !options.NoInteractsh {
		interactshClient, err := interactsh.New(&interactsh.Options{
			ServerURL:      options.InteractshURL,
			CacheSize:      int64(options.InteractionsCacheSize),
			Eviction:       time.Duration(options.InteractionsEviction) * time.Second,
			ColldownPeriod: time.Duration(options.InteractionsColldownPeriod) * time.Second,
			PollDuration:   time.Duration(options.InteractionsPollDuration) * time.Second,
			Output:         runner.output,
			IssuesClient:   runner.issuesClient,
			Progress:       runner.progress,
		})
		if err != nil {
			gologger.Error().Msgf("Could not create interactsh client: %s", err)
		} else {
			runner.interactsh = interactshClient
		}
	}

	if options.RateLimit > 0 {
		runner.ratelimiter = ratelimit.New(options.RateLimit)
	} else {
		runner.ratelimiter = ratelimit.NewUnlimited()
	}
	return runner, nil
}

// Close releases all the resources and cleans up
func (r *Runner) Close() {
	if r.output != nil {
		r.output.Close()
	}
	r.hostMap.Close()
	if r.projectFile != nil {
		r.projectFile.Close()
	}
	protocolinit.Close()
}

// RunEnumeration sets up the input layer for giving input nuclei.
// binary and runs the actual enumeration
func (r *Runner) RunEnumeration() {
	defer r.Close()

	// If we have no templates, run on whole template directory with provided tags
	if len(r.options.Templates) == 0 && len(r.options.Workflows) == 0 && !r.options.NewTemplates && (len(r.options.Tags) > 0 || len(r.options.ExcludeTags) > 0) {
		r.options.Templates = append(r.options.Templates, r.options.TemplatesDirectory)
	}
	if r.options.NewTemplates {
		templatesLoaded, err := r.readNewTemplatesFile()
		if err != nil {
			gologger.Warning().Msgf("Could not get newly added templates: %s\n", err)
		}
		r.options.Templates = append(r.options.Templates, templatesLoaded...)
	}
	includedTemplates := r.catalog.GetTemplatesPath(r.options.Templates, false)
	excludedTemplates := r.catalog.GetTemplatesPath(r.options.ExcludedTemplates, true)
	// defaults to all templates
	allTemplates := includedTemplates

	if len(excludedTemplates) > 0 {
		excludedMap := make(map[string]struct{}, len(excludedTemplates))
		for _, excl := range excludedTemplates {
			excludedMap[excl] = struct{}{}
		}
		// rebuild list with only non-excluded templates
		allTemplates = []string{}

		for _, incl := range includedTemplates {
			if _, found := excludedMap[incl]; !found {
				allTemplates = append(allTemplates, incl)
			} else {
				gologger.Warning().Msgf("Excluding '%s'", incl)
			}
		}
	}

	// pre-parse all the templates, apply filters
	finalTemplates := []*templates.Template{}

	workflowPaths := r.catalog.GetTemplatesPath(r.options.Workflows, false)
	availableTemplates, _ := r.getParsedTemplatesFor(allTemplates, r.options.Severity, false)
	availableWorkflows, workflowCount := r.getParsedTemplatesFor(workflowPaths, r.options.Severity, true)

	var unclusteredRequests int64
	for _, template := range availableTemplates {
		// workflows will dynamically adjust the totals while running, as
		// it can't be know in advance which requests will be called
		if len(template.Workflows) > 0 {
			continue
		}
		unclusteredRequests += int64(template.TotalRequests) * r.inputCount
	}

	originalTemplatesCount := len(availableTemplates)
	clusterCount := 0
	clusters := clusterer.Cluster(availableTemplates)
	for _, cluster := range clusters {
		if len(cluster) > 1 && !r.options.OfflineHTTP {
			executerOpts := protocols.ExecuterOptions{
				Output:       r.output,
				Options:      r.options,
				Progress:     r.progress,
				Catalog:      r.catalog,
				RateLimiter:  r.ratelimiter,
				IssuesClient: r.issuesClient,
				Browser:      r.browser,
				ProjectFile:  r.projectFile,
				Interactsh:   r.interactsh,
			}
			clusterID := fmt.Sprintf("cluster-%s", xid.New().String())

			finalTemplates = append(finalTemplates, &templates.Template{
				ID:            clusterID,
				RequestsHTTP:  cluster[0].RequestsHTTP,
				Executer:      clusterer.NewExecuter(cluster, &executerOpts),
				TotalRequests: len(cluster[0].RequestsHTTP),
			})
			clusterCount += len(cluster)
		} else {
			finalTemplates = append(finalTemplates, cluster...)
		}
	}
	for _, workflows := range availableWorkflows {
		finalTemplates = append(finalTemplates, workflows)
	}

	var totalRequests int64
	for _, t := range finalTemplates {
		if len(t.Workflows) > 0 {
			continue
		}
		totalRequests += int64(t.TotalRequests) * r.inputCount
	}
	if totalRequests < unclusteredRequests {
		gologger.Info().Msgf("Reduced %d requests to %d (%d templates clustered)", unclusteredRequests, totalRequests, clusterCount)
	}
	templateCount := originalTemplatesCount + len(availableWorkflows)

	// 0 matches means no templates were found in directory
	if templateCount == 0 {
		gologger.Fatal().Msgf("Error, no templates were found.\n")
	}

	gologger.Info().Msgf("Using %s rules (%s templates, %s workflows)",
		r.colorizer.Bold(templateCount).String(),
		r.colorizer.Bold(templateCount-workflowCount).String(),
		r.colorizer.Bold(workflowCount).String())

	results := &atomic.Bool{}
	wgtemplates := sizedwaitgroup.New(r.options.TemplateThreads)

	// tracks global progress and captures stdout/stderr until p.Wait finishes
	r.progress.Init(r.inputCount, templateCount, totalRequests)

	for _, t := range finalTemplates {
		wgtemplates.Add()
		go func(template *templates.Template) {
			defer wgtemplates.Done()

			if len(template.Workflows) > 0 {
				results.CAS(false, r.processWorkflowWithList(template))
			} else {
				results.CAS(false, r.processTemplateWithList(template))
			}
		}(t)
	}
	wgtemplates.Wait()

	if r.interactsh != nil {
		matched := r.interactsh.Close()
		if matched {
			results.CAS(false, true)
		}
	}
	r.progress.Stop()

	if r.issuesClient != nil {
		r.issuesClient.Close()
	}
	if !results.Load() {
		gologger.Info().Msgf("No results found. Better luck next time!")
	}
	if r.browser != nil {
		r.browser.Close()
	}
}

// readNewTemplatesFile reads newly added templates from directory if it exists
func (r *Runner) readNewTemplatesFile() ([]string, error) {
	additionsFile := path.Join(r.templatesConfig.TemplatesDirectory, ".new-additions")
	file, err := os.Open(additionsFile)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	templatesList := []string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			continue
		}
		templatesList = append(templatesList, text)
	}
	return templatesList, nil
}
