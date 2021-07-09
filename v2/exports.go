package v2

import (
	"github.com/yaklang/nuclei/v2/internal/runner"
	"github.com/yaklang/nuclei/v2/pkg/types"
)

var ParseOptions = runner.ParseOptions
var New = runner.New
var Version = runner.Version

func GetDefaultOptions() *types.Options {
	opt := &types.Options{}
	ParseOptions(opt)
	return opt
}
