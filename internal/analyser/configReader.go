package analyser

import (
	"context"

	"gopkg.in/yaml.v1"

	"github.com/bradleyfalzon/gopherci/internal/db"
	"github.com/pkg/errors"
)

// RepoConfig contains the analyser configuration for the repository.
type RepoConfig struct {
	APTPackages []string `yaml:"apt_packages"`
	Tools       []db.Tool
}

// A ConfigReader returns a repository's configuration.
type ConfigReader interface {
	Read(context.Context, Executer) (RepoConfig, error)
}

// YAMLConfig implements a ConfigReader by reading a yaml configuration file
// from the repositories root.
type YAMLConfig struct {
	Tools []db.Tool // Preset tools to use, before per repo config has been applied
}

var _ ConfigReader = &YAMLConfig{}

// Read implements the ConfigReader interface.
func (c *YAMLConfig) Read(ctx context.Context, exec Executer) (RepoConfig, error) {
	cfg := RepoConfig{
		Tools: c.Tools,
	}

	const configFilename = ".gopherci.yml"

	args := []string{"cat", configFilename}
	yml, err := exec.Execute(ctx, args)
	switch err.(type) {
	case nil:
	case *NonZeroError:
		return cfg, nil
	default:
		return cfg, errors.Wrapf(err, "could not read %s", configFilename)
	}

	if err = yaml.Unmarshal(yml, &cfg); err != nil {
		return cfg, errors.Wrapf(err, "could not unmarshal %s", configFilename)
	}

	return cfg, nil
}
