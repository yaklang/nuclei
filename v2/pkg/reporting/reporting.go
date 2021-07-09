package reporting

import (
	"strings"

	"github.com/pkg/errors"
	"github.com/yaklang/nuclei/v2/pkg/output"
	"github.com/yaklang/nuclei/v2/pkg/reporting/dedupe"
	"github.com/yaklang/nuclei/v2/pkg/reporting/exporters/disk"
	"github.com/yaklang/nuclei/v2/pkg/reporting/exporters/sarif"
	"github.com/yaklang/nuclei/v2/pkg/reporting/trackers/github"
	"github.com/yaklang/nuclei/v2/pkg/reporting/trackers/gitlab"
	"github.com/yaklang/nuclei/v2/pkg/reporting/trackers/jira"
	"github.com/yaklang/nuclei/v2/pkg/types"
	"go.uber.org/multierr"
)

// Options is a configuration file for nuclei reporting module
type Options struct {
	// AllowList contains a list of allowed events for reporting module
	AllowList *Filter `yaml:"allow-list"`
	// DenyList contains a list of denied events for reporting module
	DenyList *Filter `yaml:"deny-list"`
	// Github contains configuration options for Github Issue Tracker
	Github *github.Options `yaml:"github"`
	// Gitlab contains configuration options for Gitlab Issue Tracker
	Gitlab *gitlab.Options `yaml:"gitlab"`
	// Jira contains configuration options for Jira Issue Tracker
	Jira *jira.Options `yaml:"jira"`
	// DiskExporter contains configuration options for Disk Exporter Module
	DiskExporter *disk.Options `yaml:"disk"`
	// SarifExporter contains configuration options for Sarif Exporter Module
	SarifExporter *sarif.Options `yaml:"sarif"`
}

// Filter filters the received event and decides whether to perform
// reporting for it or not.
type Filter struct {
	Severity string `yaml:"severity"`
	severity []string
	Tags     string `yaml:"tags"`
	tags     []string
}

// Compile compiles the filter creating match structures.
func (f *Filter) Compile() {
	parts := strings.Split(f.Severity, ",")
	for _, part := range parts {
		f.severity = append(f.severity, strings.TrimSpace(part))
	}
	parts = strings.Split(f.Tags, ",")
	for _, part := range parts {
		f.tags = append(f.tags, strings.TrimSpace(part))
	}
}

// GetMatch returns true if a filter matches result event
func (f *Filter) GetMatch(event *output.ResultEvent) bool {
	severity := types.ToString(event.Info["severity"])
	if len(f.severity) > 0 {
		return stringSliceContains(f.severity, severity)
	}

	tags := event.Info["tags"]
	tagParts := strings.Split(types.ToString(tags), ",")
	for i, tag := range tagParts {
		tagParts[i] = strings.TrimSpace(tag)
	}
	for _, tag := range f.tags {
		if stringSliceContains(tagParts, tag) {
			return true
		}
	}
	return false
}

// Tracker is an interface implemented by an issue tracker
type Tracker interface {
	// CreateIssue creates an issue in the tracker
	CreateIssue(event *output.ResultEvent) error
}

// Exporter is an interface implemented by an issue exporter
type Exporter interface {
	// Close closes the exporter after operation
	Close() error
	// Export exports an issue to an exporter
	Export(event *output.ResultEvent) error
}

// Client is a client for nuclei issue tracking module
type Client struct {
	trackers  []Tracker
	exporters []Exporter
	options   *Options
	dedupe    *dedupe.Storage
}

// New creates a new nuclei issue tracker reporting client
func New(options *Options, db string) (*Client, error) {
	if options.AllowList != nil {
		options.AllowList.Compile()
	}
	if options.DenyList != nil {
		options.DenyList.Compile()
	}

	client := &Client{options: options}
	if options.Github != nil {
		tracker, err := github.New(options.Github)
		if err != nil {
			return nil, errors.Wrap(err, "could not create reporting client")
		}
		client.trackers = append(client.trackers, tracker)
	}
	if options.Gitlab != nil {
		tracker, err := gitlab.New(options.Gitlab)
		if err != nil {
			return nil, errors.Wrap(err, "could not create reporting client")
		}
		client.trackers = append(client.trackers, tracker)
	}
	if options.Jira != nil {
		tracker, err := jira.New(options.Jira)
		if err != nil {
			return nil, errors.Wrap(err, "could not create reporting client")
		}
		client.trackers = append(client.trackers, tracker)
	}
	if options.DiskExporter != nil {
		exporter, err := disk.New(options.DiskExporter)
		if err != nil {
			return nil, errors.Wrap(err, "could not create exporting client")
		}
		client.exporters = append(client.exporters, exporter)
	}
	if options.SarifExporter != nil {
		exporter, err := sarif.New(options.SarifExporter)
		if err != nil {
			return nil, errors.Wrap(err, "could not create exporting client")
		}
		client.exporters = append(client.exporters, exporter)
	}
	storage, err := dedupe.New(db)
	if err != nil {
		return nil, err
	}
	client.dedupe = storage
	return client, nil
}

// Close closes the issue tracker reporting client
func (c *Client) Close() {
	c.dedupe.Close()
	for _, exporter := range c.exporters {
		exporter.Close()
	}
}

// CreateIssue creates an issue in the tracker
func (c *Client) CreateIssue(event *output.ResultEvent) error {
	if c.options.AllowList != nil && !c.options.AllowList.GetMatch(event) {
		return nil
	}
	if c.options.DenyList != nil && c.options.DenyList.GetMatch(event) {
		return nil
	}

	unique, err := c.dedupe.Index(event)
	if unique {
		for _, tracker := range c.trackers {
			if trackerErr := tracker.CreateIssue(event); trackerErr != nil {
				err = multierr.Append(err, trackerErr)
			}
		}
		for _, exporter := range c.exporters {
			if exportErr := exporter.Export(event); exportErr != nil {
				err = multierr.Append(err, exportErr)
			}
		}
	}
	return err
}

func stringSliceContains(slice []string, item string) bool {
	for _, i := range slice {
		if strings.EqualFold(i, item) {
			return true
		}
	}
	return false
}
