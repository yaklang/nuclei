package runner

import (
	"bufio"
	"errors"
	"net/url"
	"os"
	"strings"

	"github.com/projectdiscovery/gologger"
	"github.com/projectdiscovery/gologger/formatter"
	"github.com/projectdiscovery/gologger/levels"
	"github.com/yaklang/nuclei/v2/pkg/protocols/common/protocolinit"
	"github.com/yaklang/nuclei/v2/pkg/types"
)

// ParseOptions parses the command line flags provided by a user
func ParseOptions(options *types.Options) {
	// Check if stdin pipe was given
	options.Stdin = hasStdin()

	// Read the inputs and configure the logging
	configureOutput(options)

	// Show the user the banner
	showBanner()

	if options.Version {
		gologger.Info().Msgf("Current Version: %s\n", Version)
		os.Exit(0)
	}
	if options.TemplatesVersion {
		config, err := readConfiguration()
		if err != nil {
			gologger.Fatal().Msgf("Could not read template configuration: %s\n", err)
		}
		gologger.Info().Msgf("Current nuclei-templates version: %s (%s)\n", config.CurrentVersion, config.TemplatesDirectory)
		os.Exit(0)
	}

	// Validate the options passed by the user and if any
	// invalid options have been used, exit.
	if err := validateOptions(options); err != nil {
		gologger.Fatal().Msgf("Program exiting: %s\n", err)
	}

	// Auto adjust rate limits when using headless mode if the user
	// hasn't specified any custom limits.
	if options.Headless && options.BulkSize == 25 && options.TemplateThreads == 10 {
		options.BulkSize = 2
		options.TemplateThreads = 2
	}

	// Load the resolvers if user asked for them
	loadResolvers(options)

	err := protocolinit.Init(options)
	if err != nil {
		gologger.Fatal().Msgf("Could not initialize protocols: %s\n", err)
	}
}

// hasStdin returns true if we have stdin input
func hasStdin() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}

	isPipedFromChrDev := (stat.Mode() & os.ModeCharDevice) == 0
	isPipedFromFIFO := (stat.Mode() & os.ModeNamedPipe) != 0

	return isPipedFromChrDev || isPipedFromFIFO
}

// validateOptions validates the configuration options passed
func validateOptions(options *types.Options) error {
	// Both verbose and silent flags were used
	if options.Verbose && options.Silent {
		return errors.New("both verbose and silent mode specified")
	}

	if !options.TemplateList {
		// Check if a list of templates was provided and it exists
		if len(options.Templates) == 0 && !options.NewTemplates && len(options.Workflows) == 0 && len(options.Tags) == 0 && !options.UpdateTemplates {
			return errors.New("no template/templates provided")
		}
	}

	// Validate proxy options if provided
	err := validateProxyURL(options.ProxyURL, "invalid http proxy format (It should be http://username:password@host:port)")
	if err != nil {
		return err
	}

	err = validateProxyURL(options.ProxySocksURL, "invalid socks proxy format (It should be socks5://username:password@host:port)")
	if err != nil {
		return err
	}
	return nil
}

func validateProxyURL(proxyURL, message string) error {
	if proxyURL != "" && !isValidURL(proxyURL) {
		return errors.New(message)
	}

	return nil
}

func isValidURL(urlString string) bool {
	_, err := url.Parse(urlString)
	return err == nil
}

// configureOutput configures the output on the screen
func configureOutput(options *types.Options) {
	// If the user desires verbose output, show verbose output
	if options.Verbose {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelVerbose)
	}
	if options.Debug {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelDebug)
	}
	if options.NoColor {
		gologger.DefaultLogger.SetFormatter(formatter.NewCLI(true))
	}
	if options.Silent {
		gologger.DefaultLogger.SetMaxLevel(levels.LevelSilent)
	}
}

// loadResolvers loads resolvers from both user provided flag and file
func loadResolvers(options *types.Options) {
	if options.ResolversFile == "" {
		return
	}

	file, err := os.Open(options.ResolversFile)
	if err != nil {
		gologger.Fatal().Msgf("Could not open resolvers file: %s\n", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		part := scanner.Text()
		if part == "" {
			continue
		}
		if strings.Contains(part, ":") {
			options.InternalResolversList = append(options.InternalResolversList, part)
		} else {
			options.InternalResolversList = append(options.InternalResolversList, part+":53")
		}
	}
}
